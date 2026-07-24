package main

import (
	"strings"
	"testing"
)

// fixture builds a minimal `go test -json` stream: one line per event, in
// the same shape `go test -json` actually emits (only the fields this
// package reads are populated).
func fixture(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func TestParseEventsWallSeconds(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"start","Package":"pkg"}`,
		`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestFoo"}`,
		`{"Time":"2024-01-01T00:00:03Z","Action":"pass","Package":"pkg","Test":"TestFoo","Elapsed":3.0}`,
		`{"Time":"2024-01-01T00:00:05Z","Action":"pass","Package":"pkg","Elapsed":5.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 20)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if report.WallSeconds != 5.0 {
		t.Fatalf("WallSeconds = %v, want 5.0 (first to last event timestamp)", report.WallSeconds)
	}
	if report.OS != "linux" {
		t.Fatalf("OS = %q, want %q", report.OS, "linux")
	}
}

func TestParseEventsSlowestOrderingAndTruncation(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestFast","Elapsed":0.1}`,
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestSlow","Elapsed":9.0}`,
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestMedium","Elapsed":2.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 2)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(report.SlowestTests) != 2 {
		t.Fatalf("len(SlowestTests) = %d, want 2 (keepSlowest truncation)", len(report.SlowestTests))
	}
	if report.SlowestTests[0].Name != "TestSlow" || report.SlowestTests[1].Name != "TestMedium" {
		t.Fatalf("SlowestTests = %+v, want [TestSlow, TestMedium] in that order", report.SlowestTests)
	}
}

func TestParseEventsCapturesFailureOutput(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestBad"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"pkg","Test":"TestBad","Output":"--- FAIL: TestBad\n"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"pkg","Test":"TestBad","Output":"    want 1, got 2\n"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"fail","Package":"pkg","Test":"TestBad","Elapsed":1.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 20)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("len(Failures) = %d, want 1", len(report.Failures))
	}
	want := "--- FAIL: TestBad\n    want 1, got 2\n"
	if report.Failures[0].Output != want {
		t.Fatalf("Failures[0].Output = %q, want %q", report.Failures[0].Output, want)
	}
	if report.Failures[0].Name != "TestBad" || report.Failures[0].Package != "pkg" {
		t.Fatalf("Failures[0] = %+v, want Name=TestBad Package=pkg", report.Failures[0])
	}
}

func TestParseEventsSkipIsNotAFailure(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"skip","Package":"pkg","Test":"TestSkipped","Elapsed":0.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 20)
	if err != nil {
		t.Fatalf("parseEvents: %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("len(Failures) = %d, want 0 (skip is not a failure)", len(report.Failures))
	}
	if len(report.SlowestTests) != 1 {
		t.Fatalf("len(SlowestTests) = %d, want 1 (skipped tests still get a timing entry)", len(report.SlowestTests))
	}
}

func TestParseEventsRejectsMalformedInput(t *testing.T) {
	if _, err := parseEvents(strings.NewReader("not json\n"), "linux", 20); err == nil {
		t.Fatal("parseEvents accepted malformed input, want an error")
	}
}
