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
	flag.Parse()

	report, err := parseEvents(os.Stdin, *osName, *slowest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testreport: parse go test -json stream: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "testreport: encode report: %v\n", err)
		os.Exit(1)
	}
}
