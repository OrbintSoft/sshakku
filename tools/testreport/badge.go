package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// shieldsBadge is the shields.io "endpoint" badge schema: a small JSON file
// shields.io fetches and renders into an SVG on the fly, so this tool never
// needs to generate badge images itself. See
// https://shields.io/badges/endpoint-badge.
type shieldsBadge struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// badgeColor picks a shields.io color name from a coverage percentage,
// following the red/yellow/brightgreen convention common to coverage badges.
func badgeColor(percent float64) string {
	switch {
	case percent < 50:
		return "red"
	case percent < 80:
		return "yellow"
	default:
		return "brightgreen"
	}
}

// errNoCoverageData is a report that carries no per-package coverage, so there
// is no percentage to put on a badge. The OS it came from leads the sentence,
// since a run is identified by the machine it happened on.
var errNoCoverageData = errors.New("has no coverage data")

// renderBadgeJSON builds the shields.io endpoint badge JSON for r's coverage.
func renderBadgeJSON(r Report) ([]byte, error) {
	if len(r.PackageCoverage) == 0 {
		return nil, fmt.Errorf("report for %s %w", r.OS, errNoCoverageData)
	}
	badge := shieldsBadge{
		SchemaVersion: 1,
		Label:         "coverage " + r.OS,
		Message:       fmt.Sprintf("%.1f%%", r.CoveragePercent),
		Color:         badgeColor(r.CoveragePercent),
	}
	return json.MarshalIndent(badge, "", "  ")
}
