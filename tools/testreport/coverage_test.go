package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCoverageProfileTotalAndPerPackage(t *testing.T) {
	in := fixture(
		`mode: set`,
		`sshakku/internal/keys/load.go:10.1,12.2 3 1`,
		`sshakku/internal/keys/load.go:14.1,16.2 2 0`,
		`sshakku/internal/config/config.go:5.1,7.2 5 1`,
	)

	total, perPackage, err := parseCoverageProfile(strings.NewReader(in))
	require.NoError(t, err, "parseCoverageProfile")

	// keys: 3 of 5 statements covered = 60%; config: 5 of 5 = 100%.
	// total: 8 of 10 statements covered = 80%.
	assert.InDelta(t, 80.0, total, floatTolerance, "the whole profile's statements, not the average of the packages")
	assert.Equal(t, []PackageCoverage{
		{Package: "sshakku/internal/config", Percent: 100.0},
		{Package: "sshakku/internal/keys", Percent: 60.0},
	}, perPackage, "one row per package, in the order the table prints them")
}

func TestParseCoverageProfileRejectsMalformedInput(t *testing.T) {
	in := fixture(
		`mode: set`,
		`not a valid coverage line`,
	)
	_, _, err := parseCoverageProfile(strings.NewReader(in))
	assert.Error(t, err, "a profile that could not be read must be reported, not counted as zero")
}

func TestParseCoverageProfileEmptyIsZero(t *testing.T) {
	in := fixture(`mode: set`)
	total, perPackage, err := parseCoverageProfile(strings.NewReader(in))
	require.NoError(t, err, "parseCoverageProfile")
	assert.Zero(t, total, "a profile with no statements in it covers nothing")
	assert.Empty(t, perPackage, "and names no package")
}
