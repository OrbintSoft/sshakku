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

// pagesReportURL is the GitHub Pages URL (serving the coverage-reports
// branch root) for the given OS's Mochawesome-style HTML test report.
func pagesReportURL(os string) string {
	return fmt.Sprintf("https://orbintsoft.github.io/sshakku/report-%s.html", os)
}

// pagesCoverageURL is the GitHub Pages URL for the given OS's per-line HTML
// coverage report.
func pagesCoverageURL(os string) string {
	return fmt.Sprintf("https://orbintsoft.github.io/sshakku/coverage-%s.html", os)
}

// renderMarkdown formats one Report per OS into the Markdown body of the
// per-PR test-health comment: coverage and wall-clock time per OS, a link to
// each OS's HTML test report and coverage report, each OS's per-package
// coverage breakdown and slowest tests, and a failures section listing every
// failing test's captured output (omitted when nothing failed).
//
// reportURLs and coverageURLs map an OS to the link its "Test report" /
// "Coverage report" cell should use. An OS absent from a map falls back to
// the GitHub Pages site publish-coverage-report publishes to on every merge
// to master -- correct when rendering report.md itself, since that's exactly
// what gets published there. The per-PR test-health comment passes each OS's
// own workflow-run artifact URL instead, since the Pages site only ever
// reflects the last *master* merge, not this PR's own run.
func renderMarkdown(reports []Report, reportURLs, coverageURLs map[string]string) string {
	sorted := make([]Report, len(reports))
	copy(sorted, reports)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].OS < sorted[j].OS })

	var b strings.Builder
	fmt.Fprintln(&b, commentMarker)
	fmt.Fprintln(&b, "## Test health")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| OS | Coverage | Wall time | Slowest test | Test report | Coverage report |")
	fmt.Fprintln(&b, "| --- | --- | --- | --- | --- | --- |")
	for _, r := range sorted {
		coverage := "n/a"
		if len(r.PackageCoverage) > 0 {
			coverage = fmt.Sprintf("%.1f%%", r.CoveragePercent)
		}
		slowest := "n/a"
		if len(r.SlowestTests) > 0 {
			slowest = fmt.Sprintf("%s (%.2fs)", r.SlowestTests[0].Name, r.SlowestTests[0].Seconds)
		}
		testURL := pagesReportURL(r.OS)
		if u, ok := reportURLs[r.OS]; ok {
			testURL = u
		}
		coverageURL := pagesCoverageURL(r.OS)
		if u, ok := coverageURLs[r.OS]; ok {
			coverageURL = u
		}
		fmt.Fprintf(&b, "| %s | %s | %.1fs | %s | [HTML](%s) | [HTML](%s) |\n", r.OS, coverage, r.WallSeconds, slowest, testURL, coverageURL)
	}

	for _, r := range sorted {
		if len(r.PackageCoverage) == 0 {
			continue
		}
		byCoverage := make([]PackageCoverage, len(r.PackageCoverage))
		copy(byCoverage, r.PackageCoverage)
		sort.Slice(byCoverage, func(i, j int) bool { return byCoverage[i].Percent < byCoverage[j].Percent })

		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "<details><summary>Coverage by package (%s)</summary>\n\n", r.OS)
		fmt.Fprintln(&b, "| Package | Coverage |")
		fmt.Fprintln(&b, "| --- | --- |")
		for _, p := range byCoverage {
			fmt.Fprintf(&b, "| %s | %.1f%% |\n", p.Package, p.Percent)
		}
		fmt.Fprintln(&b, "\n</details>")
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
