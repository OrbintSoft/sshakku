#!/usr/bin/env bash
#
# Publishes coverage-linux.json, coverage-macos.json, and report.md (expected
# in the current directory) to the orphan coverage-reports branch, skipping
# the commit if nothing changed. Run from the repository root after those
# three files have been generated; needs contents: write and a git remote
# named origin with push access.
set -euo pipefail

worktree="$(mktemp -d)"
cleanup() {
	git worktree remove --force "$worktree" 2>/dev/null || rm -rf "$worktree"
	git worktree prune
}
trap cleanup EXIT

git fetch origin coverage-reports
git worktree add "$worktree" coverage-reports

cp coverage-linux.json coverage-macos.json report.md "$worktree/"

git -C "$worktree" add coverage-linux.json coverage-macos.json report.md
if git -C "$worktree" diff --cached --quiet; then
	echo "coverage-reports: no changes, skipping commit"
	exit 0
fi

git -C "$worktree" \
	-c user.name="github-actions[bot]" \
	-c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
	commit -m "coverage-reports: update from ${GITHUB_SHA}"
git -C "$worktree" push origin coverage-reports
