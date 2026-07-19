UNAME := $(shell uname)

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
ETC_PROFILE_D ?= /etc/profile.d/
NN ?= 001

USER_HOME ?= $(HOME)
USER_BINDIR ?= $(USER_HOME)/.local/bin

GO ?= go
GO_MAIN = ./cmd/sshakku
GO_BIN = bin/sshakku

ifeq ($(UNAME),Linux)
SSH_INIT_INSTALL_SCRIPT = nn-ssh-init-linux.sh
INSTALL_PATH = $(DESTDIR)$(BINDIR)
SSH_INIT_NAME= $(NN)-ssh-init.sh
SSH_INIT_BIND_PATH = $(ETC_PROFILE_D)$(SSH_INIT_NAME)
SSH_INIT_INSTALL_PATH = $(DESTDIR)$(SSH_INIT_BIND_PATH)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku
SSHAKKU_RUNTIME_PATH = $(BINDIR)/sshakku

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@install -Dm755 $(GO_BIN) $(SSHAKKU_INSTALL_PATH)
	@echo "Installing $(SSH_INIT_INSTALL_SCRIPT) to $(SSH_INIT_INSTALL_PATH)"
	@install -Dm755 $(SSH_INIT_INSTALL_SCRIPT) $(SSH_INIT_INSTALL_PATH)
	@echo "Setting binary paths in $(SSH_INIT_INSTALL_PATH)"
	@sed -i 's|/usr/local/bin/sshakku|$(SSHAKKU_RUNTIME_PATH)|g' $(SSH_INIT_INSTALL_PATH)
	@echo "Installation complete."

uninstall:
	@echo "Uninstalling $(SSHAKKU_INSTALL_PATH)"
	@rm -f $(SSHAKKU_INSTALL_PATH)
	@echo "Uninstalling $(SSH_INIT_INSTALL_PATH)"
	@rm -f $(SSH_INIT_INSTALL_PATH)
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(USER_BINDIR)/sshakku"
	@install -Dm755 $(GO_BIN) $(USER_BINDIR)/sshakku
	@echo "Wiring the per-user login hook"
	@./install-user-hook.sh install "$(USER_HOME)" "$(USER_BINDIR)/sshakku" "$(NN)"
	@echo "Installation complete."

uninstall-user:
	@echo "Uninstalling $(USER_BINDIR)/sshakku"
	@rm -f $(USER_BINDIR)/sshakku
	@echo "Removing the per-user login hook"
	@./install-user-hook.sh uninstall "$(USER_HOME)" "$(NN)"
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

# Shell-level login-hook and agent-lifecycle regression suite. Requires
# bats-core; only safe in a disposable environment (tier 1's own container
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

# Linting. Requires: shellcheck, shfmt, markdownlint-cli2, taplo, checkmake,
# actionlint, editorconfig-checker, hadolint, zsh. Each tool reads its own
# config file where it has one.
SH_SCRIPTS = $(wildcard *.sh) $(wildcard .githooks/*) $(wildcard test/containers/*.sh) $(wildcard test/bats/*.bats) $(wildcard test/bats/*.bash) $(wildcard test/bats/fixtures/*)
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

.PHONY: install uninstall install-user uninstall-user build test test-bats print-paths lint lint-sh lint-zsh lint-md lint-toml lint-make lint-yaml lint-editorconfig lint-go lint-docker
.DEFAULT_GOAL := install
