package main

import (
	"fmt"
	"sort"
	"strings"
)

// commentMarker identifies the sshakku test-health comment on a PR so CI can
// find and update its own prior comment instead of posting a new one every
// run. Must be the first line of the rendered body.
const commentMarker = "<!-- sshakku:test-health-report -->"

// renderMarkdown formats one Report per OS into the Markdown body of the
// per-PR test-health comment: coverage and wall-clock time per OS, each OS's
// slowest tests, and a failures section listing every failing test's
// captured output (omitted when nothing failed).
func renderMarkdown(reports []Report) string {
	sorted := make([]Report, len(reports))
	copy(sorted, reports)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].OS < sorted[j].OS })

	var b strings.Builder
	fmt.Fprintln(&b, commentMarker)
	fmt.Fprintln(&b, "## Test health")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| OS | Coverage | Wall time | Slowest test |")
	fmt.Fprintln(&b, "| --- | --- | --- | --- |")
	for _, r := range sorted {
		coverage := "n/a"
		if len(r.PackageCoverage) > 0 {
			coverage = fmt.Sprintf("%.1f%%", r.CoveragePercent)
		}
		slowest := "n/a"
		if len(r.SlowestTests) > 0 {
			slowest = fmt.Sprintf("%s (%.2fs)", r.SlowestTests[0].Name, r.SlowestTests[0].Seconds)
		}
		fmt.Fprintf(&b, "| %s | %s | %.1fs | %s |\n", r.OS, coverage, r.WallSeconds, slowest)
	}

	for _, r := range sorted {
		if len(r.SlowestTests) == 0 {
			continue
		}
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "<details><summary>Slowest tests (%s)</summary>\n\n", r.OS)
		fmt.Fprintln(&b, "| Test | Package | Seconds |")
		fmt.Fprintln(&b, "| --- | --- | --- |")
		for _, t := range r.SlowestTests {
			fmt.Fprintf(&b, "| %s | %s | %.2f |\n", t.Name, t.Package, t.Seconds)
		}
		fmt.Fprintln(&b, "\n</details>")
	}

	var totalFailures int
	for _, r := range sorted {
		totalFailures += len(r.Failures)
	}
	if totalFailures > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "### Failures")
		for _, r := range sorted {
			for _, f := range r.Failures {
				fmt.Fprintln(&b)
				fmt.Fprintf(&b, "<details><summary>%s: %s/%s</summary>\n\n", r.OS, f.Package, f.Name)
				fmt.Fprintln(&b, "```")
				fmt.Fprint(&b, f.Output)
				fmt.Fprintln(&b, "```")
				fmt.Fprintln(&b, "\n</details>")
			}
		}
	}

	return b.String()
}
