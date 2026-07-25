package main

import (
	"encoding/json"
	"flag"
	"fmt"
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

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "render":
			if err := runRender(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "testreport: render: %v\n", err)
				os.Exit(1)
			}
			return
		case "badge":
			if err := runBadge(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "testreport: badge: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	runSummarize()
}

// runRender reads one Report JSON file per path in args (as produced by the
// default summarize action) and writes the combined PR comment body to
// stdout.
func runRender(args []string) error {
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
		return fmt.Errorf("usage: testreport render [-report-url os=url] [-coverage-url os=url] <report.json> [report.json ...]")
	}
	reports := make([]Report, 0, len(paths))
	for _, path := range paths {
		f, err := os.Open(path)
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
	fmt.Print(renderMarkdown(reports, reportURLs, coverageURLs))
	return nil
}

// runBadge reads one Report JSON file (as produced by the default summarize
// action) and writes its shields.io endpoint badge JSON to stdout.
func runBadge(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: testreport badge <report.json>")
	}
	f, err := os.Open(args[0])
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
	fmt.Println(string(badge))
	return nil
}

func runSummarize() {
	osName := flag.String("os", runtime.GOOS, "operating system label recorded in the report")
	slowest := flag.Int("slowest", 20, "number of slowest tests to keep")
	coverprofile := flag.String("coverprofile", "", "path to a go test -coverprofile file; omit to skip coverage")
	flag.Parse()

	report, err := parseEvents(os.Stdin, *osName, *slowest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testreport: parse go test -json stream: %v\n", err)
		os.Exit(1)
	}

	if *coverprofile != "" {
		f, err := os.Open(*coverprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "testreport: open coverage profile: %v\n", err)
			os.Exit(1)
		}
		total, perPackage, err := parseCoverageProfile(f)
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "testreport: parse coverage profile: %v\n", err)
			os.Exit(1)
		}
		report.CoveragePercent = total
		report.PackageCoverage = perPackage
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "testreport: encode report: %v\n", err)
		os.Exit(1)
	}
}
