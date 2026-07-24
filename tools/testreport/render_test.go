package main

import "testing"

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
