package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMarkdownStartsWithMarker(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	// The marker is how the workflow finds its own comment to edit instead of
	// posting a second one, so it has to be the very first thing.
	assert.True(t, strings.HasPrefix(out, commentMarker), "the comment must start with the marker")
}

func TestRenderMarkdownOrdersByOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "macos", WallSeconds: 1},
		{OS: "linux", WallSeconds: 2},
	}, nil, nil)
	// Both rows have to be there before their order means anything: comparing
	// the position of a row that is absent reads as "early enough".
	require.Contains(t, out, "| linux |", "the linux row")
	require.Contains(t, out, "| macos |", "the macos row")
	assert.Less(t, strings.Index(out, "| linux |"), strings.Index(out, "| macos |"),
		"the rows must be ordered by OS, whatever order the runs finished in")
}

func TestRenderMarkdownOmitsFailuresSectionWhenNoneFail(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	assert.NotContains(t, out, "### Failures", "a run with nothing failing has no failures to head")
}

func TestRenderMarkdownIncludesFailureOutput(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		Failures: []TestFailure{
			{Name: "TestBad", Package: "pkg", Output: "--- FAIL: TestBad\nwant 1, got 2\n"},
		},
	}}, nil, nil)
	assert.Contains(t, out, "### Failures", "the comment must head the failures")
	assert.Contains(t, out, "pkg/TestBad", "the comment must name the test that failed")
	assert.Contains(t, out, "want 1, got 2", "and carry what it said, which is the part worth reading")
}

func TestRenderMarkdownShowsCoverageOnlyWhenPresent(t *testing.T) {
	withCoverage := renderMarkdown([]Report{{
		OS:              "linux",
		CoveragePercent: 87.5,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 87.5}},
	}}, nil, nil)
	// Anchored on the summary row, not on the number appearing anywhere: the
	// same percentage is printed again in the per-package table below, so a
	// summary cell stuck on "n/a" would satisfy a looser search.
	assert.Contains(t, withCoverage, "| linux | 87.5% |", "the summary row must carry the measured percentage")

	withoutCoverage := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	assert.Contains(t, withoutCoverage, "| linux | n/a |",
		"a run that measured nothing must say so rather than show a number it does not have")
}

func TestRenderMarkdownLinksHTMLReportPerOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "linux"},
		{OS: "macos"},
	}, nil, nil)
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/report-linux.html", "the linux report link")
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/report-macos.html", "the macos report link")
}

func TestRenderMarkdownLinksCoverageReportPerOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "linux"},
		{OS: "macos"},
	}, nil, nil)
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/coverage-linux.html", "the linux coverage link")
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/coverage-macos.html", "the macos coverage link")
}

func TestRenderMarkdownUsesArtifactURLsWhenSet(t *testing.T) {
	reportURL := "https://github.com/OrbintSoft/sshakku/actions/runs/12345/artifacts/111"
	coverageURL := "https://github.com/OrbintSoft/sshakku/actions/runs/12345/artifacts/222"
	out := renderMarkdown(
		[]Report{{OS: "linux"}},
		map[string]string{"linux": reportURL},
		map[string]string{"linux": coverageURL},
	)
	assert.Contains(t, out, reportURL, "the test-report cell must link to the artifact it was given")
	assert.Contains(t, out, coverageURL, "the coverage cell must link to the artifact it was given")
	assert.NotContains(t, out, "orbintsoft.github.io",
		"a link to Pages alongside one to the artifact would send the reader to a run that is not this one")
}

func TestRenderMarkdownFallsBackToPagesForUnmappedOS(t *testing.T) {
	// linux gets an override; macos has none, so it must keep its Pages links.
	out := renderMarkdown(
		[]Report{{OS: "linux"}, {OS: "macos"}},
		map[string]string{"linux": "https://example/report-linux"},
		map[string]string{"linux": "https://example/coverage-linux"},
	)
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/report-macos.html",
		"an OS with no artifact URL keeps its Pages test report")
	assert.Contains(t, out, "https://orbintsoft.github.io/sshakku/coverage-macos.html",
		"an OS with no artifact URL keeps its Pages coverage report")
}

func TestRenderMarkdownPackageCoverageSortedWorstFirst(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		PackageCoverage: []PackageCoverage{
			{Package: "internal/good", Percent: 95.0},
			{Package: "internal/bad", Percent: 10.0},
		},
	}}, nil, nil)
	assert.Contains(t, out, "Coverage by package (linux)", "the section must be headed by the OS")
	// One decimal place, because that is the difference the table exists to
	// show: rounding 99.9% to 100% hides exactly the package worth looking at.
	assert.Contains(t, out, "| internal/bad | 10.0% |", "each row must carry the package's percentage to a tenth")
	require.Contains(t, out, "internal/bad", "the worse-covered package")
	require.Contains(t, out, "internal/good", "the better-covered package")
	assert.Less(t, strings.Index(out, "internal/bad"), strings.Index(out, "internal/good"),
		"the package that needs looking at must be the one read first")
}

func TestRenderMarkdownOmitsPackageCoverageWhenAbsent(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	assert.NotContains(t, out, "Coverage by package", "a run that measured nothing has no packages to list")
}

// TestRenderMarkdownOmitsSlowestSectionWhenAbsent is the companion to the
// per-package one above: a run that timed no test must not be given an empty
// "Slowest tests" table to unfold.
func TestRenderMarkdownOmitsSlowestSectionWhenAbsent(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	assert.NotContains(t, out, "Slowest tests", "a run that timed nothing has no tests to list")
}
