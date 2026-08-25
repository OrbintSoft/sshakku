package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// osURLFlag collects repeated `os=url` flag values into a map keyed by OS, so
// `render` can be told a distinct per-OS artifact link (each OS's report is a
// separate artifact with its own opaque id) for the Test/Coverage columns.
type osURLFlag map[string]string

func (f osURLFlag) String() string { return "" }

func (f osURLFlag) Set(v string) error {
	key, url, ok := strings.Cut(v, "=")
	if !ok || key == "" || url == "" {
		return fmt.Errorf("expected os=url, got %q", v)
	}
	f[key] = url
	return nil
}

// openFile opens a path for reading. It's a seam so tests can supply a reader
// whose Close fails, exercising the close-error handling a real *os.File
// almost never triggers.
var openFile = func(name string) (io.ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func main() {
	// The process entry point: a single os.Exit around the testable dispatch,
	// with nothing here to unit-test (a test can't observe os.Exit).
	//coverage:ignore
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run dispatches the subcommand in args and returns the process exit code,
// reading the go test -json stream from stdin and writing output to stdout and
// diagnostics to stderr. Kept separate from main so every path but the lone
// os.Exit is testable against in-memory streams.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "render":
			if err := runRender(args[1:], stdout); err != nil {
				_, _ = fmt.Fprintf(stderr, "testreport: render: %v\n", err)
				return 1
			}
			return 0
		case "badge":
			if err := runBadge(args[1:], stdout); err != nil {
				_, _ = fmt.Fprintf(stderr, "testreport: badge: %v\n", err)
				return 1
			}
			return 0
		}
	}
	if err := runSummarize(args, stdin, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "testreport: %v\n", err)
		return 1
	}
	return 0
}

// runRender reads one Report JSON file per path in args (as produced by the
// default summarize action) and writes the combined PR comment body to out.
func runRender(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	reportURLs := osURLFlag{}
	coverageURLs := osURLFlag{}
	fs.Var(reportURLs, "report-url", "os=url for an OS's HTML test report artifact; repeatable. Overrides the published GitHub Pages link for that OS's Test report cell")
	fs.Var(coverageURLs, "coverage-url", "os=url for an OS's HTML coverage report artifact; repeatable. Overrides the published GitHub Pages link for that OS's Coverage report cell")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		return errors.New("usage: testreport render [-report-url os=url] [-coverage-url os=url] <report.json> [report.json ...]")
	}
	reports := make([]Report, 0, len(paths))
	for _, path := range paths {
		f, err := openFile(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		var r Report
		err = json.NewDecoder(f).Decode(&r)
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		reports = append(reports, r)
	}
	if _, err := fmt.Fprint(out, renderMarkdown(reports, reportURLs, coverageURLs)); err != nil {
		return err
	}
	return nil
}

// runBadge reads one Report JSON file (as produced by the default summarize
// action) and writes its shields.io endpoint badge JSON to out.
func runBadge(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: testreport badge <report.json>")
	}
	f, err := openFile(args[0])
	if err != nil {
		return fmt.Errorf("open %s: %w", args[0], err)
	}
	var r Report
	err = json.NewDecoder(f).Decode(&r)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("decode %s: %w", args[0], err)
	}
	badge, err := renderBadgeJSON(r)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, string(badge)); err != nil {
		return err
	}
	return nil
}

// runSummarize reads a go test -json stream from stdin and writes the
// summarized Report JSON to stdout, optionally folding in a coverage profile.
func runSummarize(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	osName := fs.String("os", runtime.GOOS, "operating system label recorded in the report")
	slowest := fs.Int("slowest", 20, "number of slowest tests to keep")
	coverprofile := fs.String("coverprofile", "", "path to a go test -coverprofile file; omit to skip coverage")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report, err := parseEvents(stdin, *osName, *slowest)
	if err != nil {
		return fmt.Errorf("parse go test -json stream: %w", err)
	}

	if *coverprofile != "" {
		f, err := openFile(*coverprofile)
		if err != nil {
			return fmt.Errorf("open coverage profile: %w", err)
		}
		total, perPackage, err := parseCoverageProfile(f)
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("parse coverage profile: %w", err)
		}
		report.CoveragePercent = total
		report.PackageCoverage = perPackage
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}
