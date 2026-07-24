UNAME := $(shell uname)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
ETC_PROFILE_D ?= /etc/profile.d/
# The non-login-shell equivalent of /etc/profile.d: some bash builds source
# every file in here for every interactive shell, login or not. Falls back
# to BASH_BASHRC_FILE (a single file, marker-delimited block) if it doesn't
# exist on this system.
BASH_BASHRC_D ?= /etc/bash/bashrc.d/
BASH_BASHRC_FILE ?= /etc/bash.bashrc
NN ?= 001
# Opt-in: also wire the login hook into a non-login shell's startup files
# (.bashrc.d/.bashrc per-user, /etc/bash/bashrc.d/ or /etc/bash.bashrc
# system-wide), so a plain new terminal tab (which often doesn't start a
# login shell) picks it up too. Off by default; set to any non-empty value
# to enable.
WIRE_BASHRC ?=

# macOS has no /etc/zprofile.d/-style drop-in convention, so the system-wide
# install renders the hook once to SHARE_DIR and upserts a marker-block
# source line into these two single files instead — /etc/zprofile for the
# login shell (mirrors ETC_PROFILE_D's role), /etc/zshrc opt-in via
# WIRE_ZSHRC for non-login interactive shells (mirrors WIRE_BASHRC).
SHARE_DIR ?= $(PREFIX)/share/sshakku/
ETC_ZPROFILE ?= /etc/zprofile
ETC_ZSHRC ?= /etc/zshrc
WIRE_ZSHRC ?=

USER_HOME ?= $(HOME)
USER_BINDIR ?= $(USER_HOME)/.local/bin
# install-user/uninstall-user shell family: "bash" (default) or "zsh". Picks
# which of WIRE_BASHRC/WIRE_ZSHRC gates the non-login rc-file wiring and which
# profile/rc file pair install-user-hook.sh targets; install-user-hook.sh
# itself always prefers an existing .d drop-in directory over the
# marker-block fallback file, whichever shell is selected.
USER_SHELL ?= bash
ifeq ($(USER_SHELL),zsh)
USER_WIRE_RC = $(WIRE_ZSHRC)
else
USER_WIRE_RC = $(WIRE_BASHRC)
endif

GO ?= go
GO_MAIN = ./cmd/sshakku
GO_BIN = bin/sshakku

ifeq ($(UNAME),Linux)
SSH_INIT_INSTALL_SCRIPT = nn-ssh-init.sh
INSTALL_PATH = $(DESTDIR)$(BINDIR)
SSH_INIT_NAME= $(NN)-ssh-init.sh
SSH_INIT_BIND_PATH = $(ETC_PROFILE_D)$(SSH_INIT_NAME)
SSH_INIT_INSTALL_PATH = $(DESTDIR)$(SSH_INIT_BIND_PATH)
SSH_INIT_BASHRC_DROPIN_PATH = $(DESTDIR)$(BASH_BASHRC_D)$(SSH_INIT_NAME)
SSH_INIT_BASHRC_FILE_PATH = $(DESTDIR)$(BASH_BASHRC_FILE)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku
SSHAKKU_RUNTIME_PATH = $(BINDIR)/sshakku

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@install -Dm755 $(GO_BIN) $(SSHAKKU_INSTALL_PATH)
	@echo "Installing $(SSH_INIT_INSTALL_SCRIPT) to $(SSH_INIT_INSTALL_PATH)"
	@install -Dm755 $(SSH_INIT_INSTALL_SCRIPT) $(SSH_INIT_INSTALL_PATH)
	@echo "Setting binary paths in $(SSH_INIT_INSTALL_PATH)"
	@sed -i 's|/usr/local/bin/sshakku|$(SSHAKKU_RUNTIME_PATH)|g' $(SSH_INIT_INSTALL_PATH)
ifneq ($(WIRE_BASHRC),)
	@if [ -d "$(BASH_BASHRC_D)" ]; then \
		echo "Wiring the non-login bash hook into $(SSH_INIT_BASHRC_DROPIN_PATH)"; \
		mkdir -p "$(dir $(SSH_INIT_BASHRC_DROPIN_PATH))"; \
		./shell-hook-lib.sh drop-in "$(SSH_INIT_BASHRC_DROPIN_PATH)" '. "$(SSH_INIT_BIND_PATH)"'; \
	else \
		echo "Wiring the non-login bash hook into $(SSH_INIT_BASHRC_FILE_PATH)"; \
		mkdir -p "$(dir $(SSH_INIT_BASHRC_FILE_PATH))"; \
		./shell-hook-lib.sh upsert-block "$(SSH_INIT_BASHRC_FILE_PATH)" '. "$(SSH_INIT_BIND_PATH)"'; \
	fi
endif
	@echo "Installation complete."

uninstall:
	@echo "Uninstalling $(SSHAKKU_INSTALL_PATH)"
	@rm -f $(SSHAKKU_INSTALL_PATH)
	@echo "Uninstalling $(SSH_INIT_INSTALL_PATH)"
	@rm -f $(SSH_INIT_INSTALL_PATH)
	@./shell-hook-lib.sh remove-drop-in "$(SSH_INIT_BASHRC_DROPIN_PATH)"
	@if [ -f "$(SSH_INIT_BASHRC_FILE_PATH)" ]; then \
		tmp=$$(mktemp "$(SSH_INIT_BASHRC_FILE_PATH).XXXXXX"); \
		./shell-hook-lib.sh strip-block "$(SSH_INIT_BASHRC_FILE_PATH)" >"$$tmp"; \
		mv "$$tmp" "$(SSH_INIT_BASHRC_FILE_PATH)"; \
	fi
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(USER_BINDIR)/sshakku"
	@install -Dm755 $(GO_BIN) $(USER_BINDIR)/sshakku
	@echo "Wiring the per-user login hook"
	@./install-user-hook.sh install "$(USER_HOME)" "$(USER_BINDIR)/sshakku" "$(NN)" "$(USER_WIRE_RC)" "$(USER_SHELL)"
	@echo "Installation complete."

uninstall-user:
	@echo "Uninstalling $(USER_BINDIR)/sshakku"
	@rm -f $(USER_BINDIR)/sshakku
	@echo "Removing the per-user login hook"
	@./install-user-hook.sh uninstall "$(USER_HOME)" "$(NN)" "$(USER_SHELL)"
	@echo "Uninstallation complete."

else ifeq ($(UNAME),Darwin)
SSH_INIT_INSTALL_SCRIPT = nn-ssh-init.sh
INSTALL_PATH = $(DESTDIR)$(BINDIR)
SSH_INIT_NAME = $(NN)-sshakku-init.sh
SSH_INIT_HOOK_RENDERED_PATH = $(DESTDIR)$(SHARE_DIR)$(SSH_INIT_NAME)
SSH_INIT_ZPROFILE_PATH = $(DESTDIR)$(ETC_ZPROFILE)
SSH_INIT_ZSHRC_PATH = $(DESTDIR)$(ETC_ZSHRC)
# print-paths' one shared name for "where the login shell picks this up" —
# on Darwin that's the marker-block file, not the rendered hook itself.
SSH_INIT_INSTALL_PATH = $(SSH_INIT_ZPROFILE_PATH)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku
SSHAKKU_RUNTIME_PATH = $(BINDIR)/sshakku

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@mkdir -p "$(dir $(SSHAKKU_INSTALL_PATH))"
	@install -m755 $(GO_BIN) $(SSHAKKU_INSTALL_PATH)
	@echo "Rendering $(SSH_INIT_INSTALL_SCRIPT) to $(SSH_INIT_HOOK_RENDERED_PATH)"
	@mkdir -p "$(dir $(SSH_INIT_HOOK_RENDERED_PATH))"
	@install -m755 $(SSH_INIT_INSTALL_SCRIPT) $(SSH_INIT_HOOK_RENDERED_PATH)
	@sed -i '' 's|/usr/local/bin/sshakku|$(SSHAKKU_RUNTIME_PATH)|g' $(SSH_INIT_HOOK_RENDERED_PATH)
	@echo "Wiring the login hook into $(SSH_INIT_ZPROFILE_PATH)"
	@mkdir -p "$(dir $(SSH_INIT_ZPROFILE_PATH))"
	@./shell-hook-lib.sh upsert-block "$(SSH_INIT_ZPROFILE_PATH)" '. "$(SSH_INIT_HOOK_RENDERED_PATH)"'
ifneq ($(WIRE_ZSHRC),)
	@echo "Wiring the non-login zsh hook into $(SSH_INIT_ZSHRC_PATH)"
	@mkdir -p "$(dir $(SSH_INIT_ZSHRC_PATH))"
	@./shell-hook-lib.sh upsert-block "$(SSH_INIT_ZSHRC_PATH)" '. "$(SSH_INIT_HOOK_RENDERED_PATH)"'
endif
	@echo "Installation complete."

uninstall:
	@echo "Uninstalling $(SSHAKKU_INSTALL_PATH)"
	@rm -f $(SSHAKKU_INSTALL_PATH)
	@echo "Removing $(SSH_INIT_HOOK_RENDERED_PATH)"
	@rm -f $(SSH_INIT_HOOK_RENDERED_PATH)
	@rmdir "$(dir $(SSH_INIT_HOOK_RENDERED_PATH))" 2>/dev/null || true
	@if [ -f "$(SSH_INIT_ZPROFILE_PATH)" ]; then \
		tmp=$$(mktemp "$(SSH_INIT_ZPROFILE_PATH).XXXXXX"); \
		./shell-hook-lib.sh strip-block "$(SSH_INIT_ZPROFILE_PATH)" >"$$tmp"; \
		mv "$$tmp" "$(SSH_INIT_ZPROFILE_PATH)"; \
	fi
	@if [ -f "$(SSH_INIT_ZSHRC_PATH)" ]; then \
		tmp=$$(mktemp "$(SSH_INIT_ZSHRC_PATH).XXXXXX"); \
		./shell-hook-lib.sh strip-block "$(SSH_INIT_ZSHRC_PATH)" >"$$tmp"; \
		mv "$$tmp" "$(SSH_INIT_ZSHRC_PATH)"; \
	fi
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(USER_BINDIR)/sshakku"
	@mkdir -p "$(USER_BINDIR)"
	@install -m755 $(GO_BIN) $(USER_BINDIR)/sshakku
	@echo "Wiring the per-user login hook"
	@./install-user-hook.sh install "$(USER_HOME)" "$(USER_BINDIR)/sshakku" "$(NN)" "$(WIRE_ZSHRC)" zsh
	@echo "Installation complete."

uninstall-user:
	@echo "Uninstalling $(USER_BINDIR)/sshakku"
	@rm -f $(USER_BINDIR)/sshakku
	@echo "Removing the per-user login hook"
	@./install-user-hook.sh uninstall "$(USER_HOME)" "$(NN)" zsh
	@echo "Uninstallation complete."

else

install uninstall install-user uninstall-user:
	@echo "$(UNAME) is not supported."
	@exit 1
endif

build:
	$(GO) build -o $(GO_BIN) $(GO_MAIN)

test:
	$(GO) test -race ./...

# CI-only variant of test: same run, but captures a `go test -json` event
# stream and a coverage profile for tools/testreport to summarize into the
# per-PR test-health comment. Redirecting (not piping) to test.json preserves
# go test's exit status, so `make test-json` still fails the build on a test
# failure like plain `make test` does.
test-json:
	$(GO) test -race -json -coverprofile=coverage.out ./... > test.json

# Shell-level login-hook and agent-lifecycle regression suite. Requires
# bats-core; only safe in a disposable environment (the container test suite
# runs it in CI) — see test/bats/helpers.bash for the explicit opt-in gate.
test-bats:
	bats test/bats

print-paths:
	@echo "PREFIX: $(PREFIX)"
	@echo "BINDIR: $(BINDIR)"
	@echo "DESTDIR: $(DESTDIR)"
	@echo "SSHAKKU_INSTALL_PATH: $(SSHAKKU_INSTALL_PATH)"
	@echo "SSHAKKU_RUNTIME_PATH: $(SSHAKKU_RUNTIME_PATH)"
	@echo "SSH_INIT_INSTALL_PATH: $(SSH_INIT_INSTALL_PATH)"
	@echo "USER_HOME: $(USER_HOME)"
	@echo "USER_BINDIR: $(USER_BINDIR)"
	@echo "USER_SHELL: $(USER_SHELL)"

# Linting. Requires: shellcheck, shfmt, markdownlint-cli2, taplo, checkmake,
# actionlint, editorconfig-checker, hadolint, zsh. Each tool reads its own
# config file where it has one.
SH_SCRIPTS = $(wildcard *.sh) $(wildcard .githooks/*) $(wildcard .github/scripts/*.sh) $(wildcard test/*.sh) $(wildcard test/containers/*.sh) $(wildcard test/bats/*.bats) $(wildcard test/bats/*.bash) $(wildcard test/bats/fixtures/*)
ZSH_SCRIPTS = $(wildcard *.zsh)
DOCKERFILES = $(wildcard test/containers/*.Dockerfile)

lint: lint-sh lint-zsh lint-md lint-toml lint-make lint-yaml lint-editorconfig lint-go lint-docker

lint-sh:
	shellcheck $(SH_SCRIPTS)
	shfmt -d $(SH_SCRIPTS)

# zsh has no shellcheck/shfmt-equivalent linter; -n gives a real but
# syntax-only check (no style/portability warnings).
lint-zsh:
	@for f in $(ZSH_SCRIPTS); do zsh -n "$$f" || exit 1; done

lint-md:
	markdownlint-cli2

lint-toml:
	taplo lint
	taplo format --check

lint-make:
	checkmake --config=checkmake.ini Makefile

lint-yaml:
	actionlint

lint-editorconfig:
	editorconfig-checker

lint-go:
	@gofmt_out=$$(gofmt -l .); [ -z "$$gofmt_out" ] || { echo "gofmt needed on:"; echo "$$gofmt_out"; exit 1; }
	$(GO) vet ./...
	golangci-lint run

lint-docker:
	hadolint $(DOCKERFILES)

.PHONY: install uninstall install-user uninstall-user build test test-json test-bats print-paths lint lint-sh lint-zsh lint-md lint-toml lint-make lint-yaml lint-editorconfig lint-go lint-docker
.DEFAULT_GOAL := install
