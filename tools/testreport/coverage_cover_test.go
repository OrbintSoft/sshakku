package main

import (
	"errors"
	"strings"
	"testing"
)

// errReader fails on the first read, so parseCoverageProfile's scanner-error
// path is exercised rather than a clean EOF.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestParseCoverageProfileSkipsBlankLines(t *testing.T) {
	in := fixture(
		`mode: set`,
		``,
		`sshakku/pkg/x.go:1.1,2.2 3 1`,
	)
	total, _, err := parseCoverageProfile(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseCoverageProfile: %v", err)
	}
	if total != 100.0 {
		t.Fatalf("total = %v, want 100", total)
	}
}

func TestParseCoverageProfileRejectsBadFieldCount(t *testing.T) {
	// A line with a position but only two fields after it, not three.
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 3`)
	if _, _, err := parseCoverageProfile(strings.NewReader(in)); err == nil {
		t.Fatal("expected an error for the wrong field count")
	}
}

func TestParseCoverageProfileRejectsBadStatementCount(t *testing.T) {
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 notanumber 1`)
	if _, _, err := parseCoverageProfile(strings.NewReader(in)); err == nil {
		t.Fatal("expected an error for a non-numeric statement count")
	}
}

func TestParseCoverageProfileRejectsBadHitCount(t *testing.T) {
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 3 notanumber`)
	if _, _, err := parseCoverageProfile(strings.NewReader(in)); err == nil {
		t.Fatal("expected an error for a non-numeric hit count")
	}
}

func TestParseCoverageProfileReportsReadErrors(t *testing.T) {
	if _, _, err := parseCoverageProfile(errReader{}); err == nil {
		t.Fatal("expected the scanner read error to surface")
	}
}
