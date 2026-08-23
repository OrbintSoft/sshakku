#!/usr/bin/env bash
#
# Renders the sshakku test-health PR comment to stdout, with each OS's
# Test report / Coverage report cell linking to that OS's HTML artifact from
# this same workflow run (not the GitHub Pages site, which only reflects the
# last master merge). Artifacts get an opaque id only after upload, so their
# ids are resolved here via the API. Takes the per-OS report JSON files as
# arguments (e.g. report-linux.json report-macos.json report-windows.json).
# Needs the gh CLI authenticated (GH_TOKEN), and GITHUB_SERVER_URL/
# GITHUB_REPOSITORY/GITHUB_RUN_ID set (GitHub Actions sets these by default).
set -euo pipefail

# artifact_url NAME -> the browser download URL of the named artifact in this
# run. The download page redirects to a signed zip; a GitHub artifact is
# always a zip, so this downloads the report rather than rendering it inline.
artifact_url() {
	local name="$1" id
	id=$(gh api "repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}/artifacts" \
		--jq "first(.artifacts[] | select(.name == \"${name}\") | .id)")
	printf '%s/%s/actions/runs/%s/artifacts/%s' \
		"${GITHUB_SERVER_URL}" "${GITHUB_REPOSITORY}" "${GITHUB_RUN_ID}" "${id}"
}

go run ./tools/testreport render \
	-report-url "linux=$(artifact_url report-html-linux)" \
	-report-url "macos=$(artifact_url report-html-macos)" \
	-report-url "windows=$(artifact_url report-html-windows)" \
	-coverage-url "linux=$(artifact_url coverage-linux)" \
	-coverage-url "macos=$(artifact_url coverage-macos)" \
	-coverage-url "windows=$(artifact_url coverage-windows)" \
	"$@"
