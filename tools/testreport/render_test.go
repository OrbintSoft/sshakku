package main

import (
	"strings"
	"testing"
)

func TestRenderMarkdownStartsWithMarker(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}})
	if !hasPrefix(out, commentMarker) {
		t.Fatalf("renderMarkdown output does not start with the comment marker:\n%s", out)
	}
}

func TestRenderMarkdownOrdersByOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "macos", WallSeconds: 1},
		{OS: "linux", WallSeconds: 2},
	})
	if indexOf(out, "| linux |") > indexOf(out, "| macos |") {
		t.Fatalf("expected linux row before macos row, got:\n%s", out)
	}
}

func TestRenderMarkdownOmitsFailuresSectionWhenNoneFail(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}})
	if contains(out, "### Failures") {
		t.Fatalf("expected no Failures section, got:\n%s", out)
	}
}

func TestRenderMarkdownIncludesFailureOutput(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		Failures: []TestFailure{
			{Name: "TestBad", Package: "pkg", Output: "--- FAIL: TestBad\nwant 1, got 2\n"},
		},
	}})
	if !contains(out, "### Failures") {
		t.Fatalf("expected a Failures section, got:\n%s", out)
	}
	if !contains(out, "pkg/TestBad") {
		t.Fatalf("expected the failing test to be named, got:\n%s", out)
	}
	if !contains(out, "want 1, got 2") {
		t.Fatalf("expected the captured failure output, got:\n%s", out)
	}
}

func TestRenderMarkdownShowsCoverageOnlyWhenPresent(t *testing.T) {
	withCoverage := renderMarkdown([]Report{{
		OS:              "linux",
		CoveragePercent: 87.5,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 87.5}},
	}})
	if !contains(withCoverage, "87.5%") {
		t.Fatalf("expected coverage percentage in output, got:\n%s", withCoverage)
	}

	withoutCoverage := renderMarkdown([]Report{{OS: "linux"}})
	if !contains(withoutCoverage, "n/a") {
		t.Fatalf("expected n/a placeholder when coverage wasn't computed, got:\n%s", withoutCoverage)
	}
}

func TestRenderMarkdownLinksFullReport(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}})
	if !contains(out, "https://github.com/OrbintSoft/sshakku/blob/coverage-reports/report.md") {
		t.Fatalf("expected a link to the full coverage report, got:\n%s", out)
	}
}

func TestRenderMarkdownLinksHTMLReportPerOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "linux"},
		{OS: "macos"},
	})
	if !contains(out, "https://orbintsoft.github.io/sshakku/report-linux.html") {
		t.Fatalf("expected a link to the linux HTML report, got:\n%s", out)
	}
	if !contains(out, "https://orbintsoft.github.io/sshakku/report-macos.html") {
		t.Fatalf("expected a link to the macos HTML report, got:\n%s", out)
	}
}

func TestRenderMarkdownPackageCoverageSortedWorstFirst(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		PackageCoverage: []PackageCoverage{
			{Package: "internal/good", Percent: 95.0},
			{Package: "internal/bad", Percent: 10.0},
		},
	}})
	if !contains(out, "Coverage by package (linux)") {
		t.Fatalf("expected a per-package coverage section, got:\n%s", out)
	}
	badIdx := strings.Index(out, "internal/bad")
	goodIdx := strings.Index(out, "internal/good")
	if badIdx == -1 || goodIdx == -1 || badIdx > goodIdx {
		t.Fatalf("expected the worse-covered package listed first, got:\n%s", out)
	}
}

func TestRenderMarkdownOmitsPackageCoverageWhenAbsent(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}})
	if contains(out, "Coverage by package") {
		t.Fatalf("expected no per-package coverage section without data, got:\n%s", out)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
