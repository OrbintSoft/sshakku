#!/usr/bin/env bash
#
# Publishes report.md, and one coverage badge JSON, one HTML test report and
# one HTML coverage report per OS (all expected in the current directory), to
# the orphan coverage-reports branch, skipping the commit if nothing changed.
# The OSes are the ones named in published_files below. Run from
# the repository root after those files have been generated; needs
# contents: write and a git remote named origin with push access.
set -euo pipefail

published_files="report.md coverage-linux.json coverage-macos.json coverage-windows.json report-linux.html report-macos.html report-windows.html coverage-linux.html coverage-macos.html coverage-windows.html"

worktree="$(mktemp -d)"
cleanup() {
	git worktree remove --force "$worktree" 2>/dev/null || rm -rf "$worktree"
	git worktree prune
}
trap cleanup EXIT

git fetch origin coverage-reports
git worktree add "$worktree" coverage-reports

# .nojekyll keeps GitHub Pages from running the branch's JSON/Markdown/HTML
# through Jekyll -- serve every file here as a plain static asset instead.
touch "$worktree/.nojekyll"
# shellcheck disable=SC2086 # published_files is a deliberate word-split list of filenames
cp $published_files "$worktree/"

# shellcheck disable=SC2086 # same as above
git -C "$worktree" add .nojekyll $published_files
if git -C "$worktree" diff --cached --quiet; then
	echo "coverage-reports: no changes, skipping commit"
	exit 0
fi

git -C "$worktree" \
	-c user.name="github-actions[bot]" \
	-c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
	commit -m "coverage-reports: update from ${GITHUB_SHA}"
git -C "$worktree" push origin coverage-reports
