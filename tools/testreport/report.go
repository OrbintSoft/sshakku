// Package main implements testreport, a CI-only helper that turns a `go test
// -json` stream into a compact summary for the per-PR test-health comment:
// wall-clock time, the slowest tests, and any failures with their captured
// output. It is never built into the release binary (make build only builds
// cmd/sshakku) and never reimplements `go test` itself — it only summarizes
// the event stream `go test -json` already emits.
package main

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"time"
)

// testEvent mirrors one line of `go test -json` output. Only the fields this
// tool consumes are declared; see `go help test-json` (or
// cmd/internal/test2json in the Go toolchain source) for the full schema.
type testEvent struct {
	Time    time.Time
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

// TestTiming is one test's wall-clock duration.
type TestTiming struct {
	Name    string  `json:"name"`
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
}

// TestFailure is one failed test's name and its captured output.
type TestFailure struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Output  string `json:"output"`
}

// Report summarizes one `go test -json` run for one operating system.
type Report struct {
	OS              string            `json:"os"`
	WallSeconds     float64           `json:"wall_seconds"`
	CoveragePercent float64           `json:"coverage_percent,omitempty"`
	PackageCoverage []PackageCoverage `json:"package_coverage,omitempty"`
	SlowestTests    []TestTiming      `json:"slowest_tests"`
	Failures        []TestFailure     `json:"failures"`
}

// outputKey identifies one running test for buffering its interleaved output
// lines until its pass/fail/skip event arrives.
type outputKey struct {
	pkg  string
	test string
}

// parseEvents reads a `go test -json` stream from r and summarizes it into a
// Report for osName, keeping only the slowest keepSlowest tests.
func parseEvents(r io.Reader, osName string, keepSlowest int) (Report, error) {
	fold := eventFold{
		report:  Report{OS: osName},
		outputs: make(map[outputKey]*[]byte),
	}
	dec := json.NewDecoder(bufio.NewReader(r))
	for dec.More() {
		var ev testEvent
		if err := dec.Decode(&ev); err != nil {
			return Report{}, err
		}
		fold.observe(ev)
	}
	return fold.done(keepSlowest), nil
}

// eventFold is the report being built as the stream is read: the timings and
// failures gathered so far, the first and last timestamps seen, and each running
// test's output lines held until that test's own result event arrives.
type eventFold struct {
	report    Report
	outputs   map[outputKey]*[]byte
	first     time.Time
	last      time.Time
	haveFirst bool
}

// observe folds one event in.
func (f *eventFold) observe(ev testEvent) {
	if !ev.Time.IsZero() {
		if !f.haveFirst {
			f.first, f.haveFirst = ev.Time, true
		}
		f.last = ev.Time
	}

	if ev.Test == "" {
		return // package-level event, not one test's result.
	}
	key := outputKey{pkg: ev.Package, test: ev.Test}

	switch ev.Action {
	case "output":
		buf := f.outputs[key]
		if buf == nil {
			buf = new([]byte)
			f.outputs[key] = buf
		}
		*buf = append(*buf, ev.Output...)
	case "pass", "fail", "skip":
		f.report.SlowestTests = append(f.report.SlowestTests, TestTiming{
			Name:    ev.Test,
			Package: ev.Package,
			Seconds: ev.Elapsed,
		})
		if ev.Action == "fail" {
			output := ""
			if buf := f.outputs[key]; buf != nil {
				output = string(*buf)
			}
			f.report.Failures = append(f.report.Failures, TestFailure{
				Name:    ev.Test,
				Package: ev.Package,
				Output:  output,
			})
		}
		delete(f.outputs, key)
	}
}

// done closes the report: the wall time between the first and last event, and
// the slowest keepSlowest tests, slowest first.
func (f *eventFold) done(keepSlowest int) Report {
	if f.haveFirst {
		f.report.WallSeconds = f.last.Sub(f.first).Seconds()
	}
	sort.SliceStable(f.report.SlowestTests, func(i, j int) bool {
		return f.report.SlowestTests[i].Seconds > f.report.SlowestTests[j].Seconds
	})
	if len(f.report.SlowestTests) > keepSlowest {
		f.report.SlowestTests = f.report.SlowestTests[:keepSlowest]
	}
	return f.report
}
