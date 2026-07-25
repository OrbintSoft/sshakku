package main

import (
	"strings"
	"testing"
)

func TestRenderMarkdownStartsWithMarker(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	if !hasPrefix(out, commentMarker) {
		t.Fatalf("renderMarkdown output does not start with the comment marker:\n%s", out)
	}
}

func TestRenderMarkdownOrdersByOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "macos", WallSeconds: 1},
		{OS: "linux", WallSeconds: 2},
	}, nil, nil)
	if indexOf(out, "| linux |") > indexOf(out, "| macos |") {
		t.Fatalf("expected linux row before macos row, got:\n%s", out)
	}
}

func TestRenderMarkdownOmitsFailuresSectionWhenNoneFail(t *testing.T) {
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
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
	}}, nil, nil)
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
	}}, nil, nil)
	if !contains(withCoverage, "87.5%") {
		t.Fatalf("expected coverage percentage in output, got:\n%s", withCoverage)
	}

	withoutCoverage := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
	if !contains(withoutCoverage, "n/a") {
		t.Fatalf("expected n/a placeholder when coverage wasn't computed, got:\n%s", withoutCoverage)
	}
}

func TestRenderMarkdownLinksHTMLReportPerOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "linux"},
		{OS: "macos"},
	}, nil, nil)
	if !contains(out, "https://orbintsoft.github.io/sshakku/report-linux.html") {
		t.Fatalf("expected a link to the linux HTML report, got:\n%s", out)
	}
	if !contains(out, "https://orbintsoft.github.io/sshakku/report-macos.html") {
		t.Fatalf("expected a link to the macos HTML report, got:\n%s", out)
	}
}

func TestRenderMarkdownLinksCoverageReportPerOS(t *testing.T) {
	out := renderMarkdown([]Report{
		{OS: "linux"},
		{OS: "macos"},
	}, nil, nil)
	if !contains(out, "https://orbintsoft.github.io/sshakku/coverage-linux.html") {
		t.Fatalf("expected a link to the linux coverage report, got:\n%s", out)
	}
	if !contains(out, "https://orbintsoft.github.io/sshakku/coverage-macos.html") {
		t.Fatalf("expected a link to the macos coverage report, got:\n%s", out)
	}
}

func TestRenderMarkdownUsesArtifactURLsWhenSet(t *testing.T) {
	reportURL := "https://github.com/OrbintSoft/sshakku/actions/runs/12345/artifacts/111"
	coverageURL := "https://github.com/OrbintSoft/sshakku/actions/runs/12345/artifacts/222"
	out := renderMarkdown(
		[]Report{{OS: "linux"}},
		map[string]string{"linux": reportURL},
		map[string]string{"linux": coverageURL},
	)
	if !contains(out, reportURL) {
		t.Fatalf("expected the Test report cell to link to the given artifact URL, got:\n%s", out)
	}
	if !contains(out, coverageURL) {
		t.Fatalf("expected the Coverage report cell to link to the given artifact URL, got:\n%s", out)
	}
	if contains(out, "orbintsoft.github.io") {
		t.Fatalf("expected no Pages links when artifact URLs are set, got:\n%s", out)
	}
}

func TestRenderMarkdownFallsBackToPagesForUnmappedOS(t *testing.T) {
	// linux gets an override; macos has none, so it must keep its Pages links.
	out := renderMarkdown(
		[]Report{{OS: "linux"}, {OS: "macos"}},
		map[string]string{"linux": "https://example/report-linux"},
		map[string]string{"linux": "https://example/coverage-linux"},
	)
	if !contains(out, "https://orbintsoft.github.io/sshakku/report-macos.html") {
		t.Fatalf("expected macos to fall back to its Pages test report, got:\n%s", out)
	}
	if !contains(out, "https://orbintsoft.github.io/sshakku/coverage-macos.html") {
		t.Fatalf("expected macos to fall back to its Pages coverage report, got:\n%s", out)
	}
}

func TestRenderMarkdownPackageCoverageSortedWorstFirst(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		PackageCoverage: []PackageCoverage{
			{Package: "internal/good", Percent: 95.0},
			{Package: "internal/bad", Percent: 10.0},
		},
	}}, nil, nil)
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
	out := renderMarkdown([]Report{{OS: "linux"}}, nil, nil)
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
