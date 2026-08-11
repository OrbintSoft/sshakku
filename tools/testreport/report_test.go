package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// floatTolerance is what "the same number" means for the percentages and
// durations this tool computes. It is far below the one decimal place any of
// them is ever printed to, so a comparison through it is exact as far as a
// reader of the report is concerned.
const floatTolerance = 1e-9

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
	require.NoError(t, err, "parseEvents")
	assert.InDelta(t, 5.0, report.WallSeconds, floatTolerance, "the wall clock from the first event to the last, not the sum of the tests")
	assert.Equal(t, "linux", report.OS, "the report must say which OS it is from")
	// The stream's last line is the package's own result: no test name, and an
	// elapsed time covering everything the package ran. Counted as a test it
	// would head the slowest-tests table on every run, under no name at all.
	assert.Equal(t, []TestTiming{{Name: "TestFoo", Package: "pkg", Seconds: 3.0}}, report.SlowestTests,
		"only what a test did is timed as a test")
}

func TestParseEventsSlowestOrderingAndTruncation(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestFast","Elapsed":0.1}`,
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestSlow","Elapsed":9.0}`,
		`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg","Test":"TestMedium","Elapsed":2.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 2)
	require.NoError(t, err, "parseEvents")
	assert.Equal(t, []TestTiming{
		{Name: "TestSlow", Package: "pkg", Seconds: 9.0},
		{Name: "TestMedium", Package: "pkg", Seconds: 2.0},
	}, report.SlowestTests, "the slowest first, cut to the number asked for")
}

func TestParseEventsCapturesFailureOutput(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg","Test":"TestBad"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"pkg","Test":"TestBad","Output":"--- FAIL: TestBad\n"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"output","Package":"pkg","Test":"TestBad","Output":"    want 1, got 2\n"}`,
		`{"Time":"2024-01-01T00:00:01Z","Action":"fail","Package":"pkg","Test":"TestBad","Elapsed":1.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 20)
	require.NoError(t, err, "parseEvents")
	require.Len(t, report.Failures, 1, "the one test that failed")
	assert.Equal(t, "TestBad", report.Failures[0].Name, "the test that failed")
	assert.Equal(t, "pkg", report.Failures[0].Package, "the package it is in")
	assert.Equal(t, "--- FAIL: TestBad\n    want 1, got 2\n", report.Failures[0].Output,
		"every output line the test emitted, in order and unabridged")
}

func TestParseEventsSkipIsNotAFailure(t *testing.T) {
	in := fixture(
		`{"Time":"2024-01-01T00:00:00Z","Action":"skip","Package":"pkg","Test":"TestSkipped","Elapsed":0.0}`,
	)

	report, err := parseEvents(strings.NewReader(in), "linux", 20)
	require.NoError(t, err, "parseEvents")
	assert.Empty(t, report.Failures, "a test that was skipped did not fail")
	assert.Len(t, report.SlowestTests, 1, "it was still a test that ran, and it is still timed")
}

func TestParseEventsRejectsMalformedInput(t *testing.T) {
	_, err := parseEvents(strings.NewReader("not json\n"), "linux", 20)
	assert.Error(t, err, "a stream that could not be read must be reported, not summarised as an empty run")
}
