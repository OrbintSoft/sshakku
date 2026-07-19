#!/usr/bin/env bash
#
# Installs every native and Go-installed lint tool `make lint` needs, pinned
# by version/commit. Run only when linting.yml's tool cache misses; expects
# SHELLCHECK_VERSION, HADOLINT_VERSION, SHFMT_COMMIT, CHECKMAKE_COMMIT,
# ACTIONLINT_COMMIT, EDITORCONFIG_CHECKER_COMMIT, GOLANGCI_LINT_COMMIT and
# RUNNER_TEMP already set in the environment (the workflow's own job-level
# env: block and GitHub Actions' runner both provide these).
set -euxo pipefail

# Pinned ShellCheck release tarball into /usr/local/bin. Extract under
# $RUNNER_TEMP so the unpacked tree stays out of the linted workspace.
curl -fsSL "https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz" | tar -xJ -C "$RUNNER_TEMP"
sudo install "${RUNNER_TEMP}/shellcheck-v${SHELLCHECK_VERSION}/shellcheck" /usr/local/bin/shellcheck

# Pinned hadolint release binary.
curl -fsSL -o "${RUNNER_TEMP}/hadolint" "https://github.com/hadolint/hadolint/releases/download/v${HADOLINT_VERSION}/hadolint-linux-x86_64"
sudo install "${RUNNER_TEMP}/hadolint" /usr/local/bin/hadolint

# Go-based tools into $(go env GOPATH)/bin, pinned by commit hash.
go install "mvdan.cc/sh/v3/cmd/shfmt@${SHFMT_COMMIT}"                                                                        # v${SHFMT_VERSION}
go install "github.com/checkmake/checkmake/cmd/checkmake@${CHECKMAKE_COMMIT}"                                                # v${CHECKMAKE_VERSION}
go install "github.com/rhysd/actionlint/cmd/actionlint@${ACTIONLINT_COMMIT}"                                                 # v${ACTIONLINT_VERSION}
go install "github.com/editorconfig-checker/editorconfig-checker/v3/cmd/editorconfig-checker@${EDITORCONFIG_CHECKER_COMMIT}" # v${EDITORCONFIG_CHECKER_VERSION}
go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_COMMIT}"                                  # v${GOLANGCI_LINT_VERSION}
