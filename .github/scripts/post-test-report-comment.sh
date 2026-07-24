#!/usr/bin/env bash
#
# Posts or updates the sshakku test-health PR comment. Reads the Markdown
# comment body from stdin (its first line must be the marker below, which
# tools/testreport's render output always includes) and takes the PR number
# as $1. Needs the gh CLI authenticated (GH_TOKEN) with pull-requests: write,
# and GITHUB_REPOSITORY set (GitHub Actions sets this by default).
set -euo pipefail

pr_number="$1"
body="$(cat)"

export SSHAKKU_REPORT_MARKER='<!-- sshakku:test-health-report -->'

existing_id=$(gh api "repos/${GITHUB_REPOSITORY}/issues/${pr_number}/comments" --paginate \
	--jq 'map(select(.body | startswith(env.SSHAKKU_REPORT_MARKER))) | .[0].id // empty')

if [ -n "$existing_id" ]; then
	printf '%s' "$body" | gh api --method PATCH "repos/${GITHUB_REPOSITORY}/issues/comments/${existing_id}" -F body=@-
else
	printf '%s' "$body" | gh api --method POST "repos/${GITHUB_REPOSITORY}/issues/${pr_number}/comments" -F body=@-
fi
