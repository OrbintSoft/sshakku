package main

import "testing"

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
		if got := badgeColor(c.percent); got != c.want {
			t.Errorf("badgeColor(%v) = %q, want %q", c.percent, got, c.want)
		}
	}
}

func TestRenderBadgeJSONFields(t *testing.T) {
	out, err := renderBadgeJSON(Report{
		OS:              "linux",
		CoveragePercent: 87.5,
		PackageCoverage: []PackageCoverage{{Package: "pkg", Percent: 87.5}},
	})
	if err != nil {
		t.Fatalf("renderBadgeJSON: %v", err)
	}
	if !contains(string(out), `"message": "87.5%"`) {
		t.Fatalf("expected the coverage percentage as message, got:\n%s", out)
	}
	if !contains(string(out), `"color": "brightgreen"`) {
		t.Fatalf("expected brightgreen for 87.5%%, got:\n%s", out)
	}
	if !contains(string(out), `"schemaVersion": 1`) {
		t.Fatalf("expected shields.io schemaVersion 1, got:\n%s", out)
	}
}

func TestRenderBadgeJSONErrorsWithoutCoverage(t *testing.T) {
	if _, err := renderBadgeJSON(Report{OS: "linux"}); err == nil {
		t.Fatal("expected an error when the report has no coverage data")
	}
}
