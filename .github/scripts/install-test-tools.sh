#!/usr/bin/env bash
#
# Installs the Go-based test tools `make test-json` and the HTML test/
# coverage report steps need, pinned by commit hash. Expects GOTESTSUM_COMMIT,
# GOPOGH_COMMIT, and GO_BETTER_HTML_COVERAGE_COMMIT already set in the
# environment (the workflow's job-level env: block provides these).
set -euxo pipefail

go install "gotest.tools/gotestsum@${GOTESTSUM_COMMIT}"                                   # v${GOTESTSUM_VERSION}
go install "github.com/medyagh/gopogh/cmd/gopogh@${GOPOGH_COMMIT}"                        # v${GOPOGH_VERSION}
go install "github.com/chmouel/go-better-html-coverage@${GO_BETTER_HTML_COVERAGE_COMMIT}" # v${GO_BETTER_HTML_COVERAGE_VERSION}
