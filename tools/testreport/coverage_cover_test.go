package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReadBoom is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errReadBoom = errors.New("read boom")

// errReader fails on the first read, so parseCoverageProfile's scanner-error
// path is exercised rather than a clean EOF.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errReadBoom }

func TestParseCoverageProfileSkipsBlankLines(t *testing.T) {
	in := fixture(
		`mode: set`,
		``,
		`sshakku/pkg/x.go:1.1,2.2 3 1`,
	)
	total, _, err := parseCoverageProfile(strings.NewReader(in))
	require.NoError(t, err, "parseCoverageProfile")
	assert.InDelta(t, 100.0, total, floatTolerance, "a blank line is not a statement that went uncovered")
}

func TestParseCoverageProfileRejectsBadFieldCount(t *testing.T) {
	// A line with a position but only two fields after it, not three.
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 3`)
	_, _, err := parseCoverageProfile(strings.NewReader(in))
	assert.Error(t, err, "a line missing a field must be reported, not read as far as it goes")
}

func TestParseCoverageProfileRejectsBadStatementCount(t *testing.T) {
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 notanumber 1`)
	_, _, err := parseCoverageProfile(strings.NewReader(in))
	assert.Error(t, err, "a statement count that is not a number must be reported")
}

func TestParseCoverageProfileRejectsBadHitCount(t *testing.T) {
	in := fixture(`mode: set`, `sshakku/pkg/x.go:1.1,2.2 3 notanumber`)
	_, _, err := parseCoverageProfile(strings.NewReader(in))
	assert.Error(t, err, "a hit count that is not a number must be reported")
}

func TestParseCoverageProfileReportsReadErrors(t *testing.T) {
	_, _, err := parseCoverageProfile(errReader{})
	assert.Error(t, err, "a profile that could not be read to the end must not be reported as complete")
}
