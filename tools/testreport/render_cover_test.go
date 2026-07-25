package main

import "testing"

func TestRenderMarkdownIncludesSlowestTests(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		SlowestTests: []TestTiming{
			{Name: "TestSlow", Package: "pkg", Seconds: 1.23},
		},
	}}, nil, nil)

	// The slowest test appears both in the summary table cell and in the
	// dedicated "Slowest tests" section.
	if !contains(out, "TestSlow (1.23s)") {
		t.Fatalf("expected the slowest test in the summary row, got:\n%s", out)
	}
	if !contains(out, "Slowest tests (linux)") {
		t.Fatalf("expected a Slowest tests section, got:\n%s", out)
	}
}
