package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
)

func main() {
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
