package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderMarkdownIncludesSlowestTests(t *testing.T) {
	out := renderMarkdown([]Report{{
		OS: "linux",
		SlowestTests: []TestTiming{
			{Name: "TestSlow", Package: "pkg", Seconds: 1.23},
		},
	}}, nil, nil)

	// The slowest test appears both in the summary table cell and in the
	// dedicated "Slowest tests" section.
	assert.Contains(t, out, "TestSlow (1.23s)", "the summary row must name the slowest test and its time")
	assert.Contains(t, out, "Slowest tests (linux)", "the report must have a section for them, headed by the OS")
	// Two decimals in the table, which is where the times are compared with
	// each other rather than merely noticed.
	assert.Contains(t, out, "| TestSlow | pkg | 1.23 |", "the row must carry the test, its package and its time")
}
