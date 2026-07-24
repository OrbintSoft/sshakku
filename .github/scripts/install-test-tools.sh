#!/usr/bin/env bash
#
# Installs the Go-based test tools `make test-json` and the HTML test-report
# step need, pinned by commit hash. Expects GOTESTSUM_COMMIT and
# GOPOGH_COMMIT already set in the environment (the workflow's job-level
# env: block provides these).
set -euxo pipefail

go install "gotest.tools/gotestsum@${GOTESTSUM_COMMIT}"            # v${GOTESTSUM_VERSION}
go install "github.com/medyagh/gopogh/cmd/gopogh@${GOPOGH_COMMIT}" # v${GOPOGH_VERSION}
