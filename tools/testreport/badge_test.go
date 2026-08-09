package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadgeColorThresholds(t *testing.T) {
	cases := []struct {
		percent float64
		want    string
	}{
		{0, "red"},
		{49.9, "red"},
		{50, "yellow"},
		{79.9, "yellow"},
		{80, "brightgreen"},
		{100, "brightgreen"},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, badgeColor(c.percent), "the badge colour at %v%%", c.percent)
	}
}

func TestRenderBadgeJSONFields(t *testing.T) {
	out, err := renderBadgeJSON(Report{
		OS:              "linux",
		CoveragePercent: 87.5,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 87.5}},
	})
	require.NoError(t, err, "renderBadgeJSON")

	// Four independent things the badge has to carry; assert, so one run names
	// every one it left out.
	badge := string(out)
	assert.Contains(t, badge, `"message": "87.5%"`, "the badge must show the coverage percentage")
	assert.Contains(t, badge, `"color": "brightgreen"`, "87.5% is in the green band")
	assert.Contains(t, badge, `"schemaVersion": 1`, "shields.io reads the schema version")
	assert.Contains(t, badge, `"label": "coverage linux"`, "the label must say which OS the number is from")
}

// TestRenderBadgeJSONColourFollowsTheNumberShown pins that the colour is
// computed from this report's own coverage and not from anything else. The
// number and the colour are the whole badge, and of the two the colour is the
// one people read at a glance: a badge saying 12.0% in brightgreen tells the
// reader the opposite of what it says.
func TestRenderBadgeJSONColourFollowsTheNumberShown(t *testing.T) {
	out, err := renderBadgeJSON(Report{
		OS:              "linux",
		CoveragePercent: 12.0,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 12.0}},
	})
	require.NoError(t, err, "renderBadgeJSON")
	assert.Contains(t, string(out), `"message": "12.0%"`, "the badge must show this report's coverage")
	assert.Contains(t, string(out), `"color": "red"`, "and must be coloured by that same number")
}

func TestRenderBadgeJSONErrorsWithoutCoverage(t *testing.T) {
	_, err := renderBadgeJSON(Report{OS: "linux"})
	assert.Error(t, err, "a run that measured no coverage must not be given a badge saying it did")
}
