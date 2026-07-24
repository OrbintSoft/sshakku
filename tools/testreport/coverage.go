package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// PackageCoverage is one package's statement coverage percentage.
type PackageCoverage struct {
	Package string  `json:"package"`
	Percent float64 `json:"percent"`
}

// parseCoverageProfile reads a Go coverage profile (as written by `go test
// -coverprofile`) and returns the overall statement coverage percentage and
// the same broken down per package. The profile format is one `mode: ...`
// header line followed by lines of
// `file:startLine.startCol,endLine.endCol numStmt count`; a block counts as
// covered when count > 0. See `go tool cover -html` / `go help testflag` for
// the format this mirrors.
func parseCoverageProfile(r io.Reader) (total float64, perPackage []PackageCoverage, err error) {
	type counts struct{ covered, total int }
	byPackage := make(map[string]*counts)
	var all counts

	sc := bufio.NewScanner(r)
	firstLine := true
	for sc.Scan() {
		line := sc.Text()
		if firstLine {
			firstLine = false
			if strings.HasPrefix(line, "mode:") {
				continue
			}
		}
		if line == "" {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			return 0, nil, fmt.Errorf("testreport: malformed coverage line (no ':'): %q", line)
		}
		file := line[:colon]
		fields := strings.Fields(line[colon+1:])
		if len(fields) != 3 {
			return 0, nil, fmt.Errorf("testreport: malformed coverage line (want 3 fields after position, got %d): %q", len(fields), line)
		}
		numStmt, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, nil, fmt.Errorf("testreport: malformed statement count %q: %w", fields[1], err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, nil, fmt.Errorf("testreport: malformed hit count %q: %w", fields[2], err)
		}

		pkg := file
		if slash := strings.LastIndexByte(file, '/'); slash >= 0 {
			pkg = file[:slash]
		}
		c := byPackage[pkg]
		if c == nil {
			c = new(counts)
			byPackage[pkg] = c
		}
		c.total += numStmt
		all.total += numStmt
		if count > 0 {
			c.covered += numStmt
			all.covered += numStmt
		}
	}
	if err := sc.Err(); err != nil {
		return 0, nil, err
	}

	if all.total > 0 {
		total = 100 * float64(all.covered) / float64(all.total)
	}
	for pkg, c := range byPackage {
		pct := 0.0
		if c.total > 0 {
			pct = 100 * float64(c.covered) / float64(c.total)
		}
		perPackage = append(perPackage, PackageCoverage{Package: pkg, Percent: pct})
	}
	sort.Slice(perPackage, func(i, j int) bool {
		return perPackage[i].Package < perPackage[j].Package
	})

	return total, perPackage, nil
}
