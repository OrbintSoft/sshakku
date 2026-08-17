UNAME := $(shell uname)

# `uname` under a POSIX-emulating environment on Windows reports the environment
# and its kernel version (MINGW64_NT-10.0-…), so the family is matched, never the
# whole string.
WINDOWS_UNAME = $(filter MINGW% MSYS%,$(UNAME))

ifeq ($(WINDOWS_UNAME),)
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
else
# Where Windows keeps programs, asked of the system rather than written down: a
# system is not necessarily installed on C:, and each program of this kind gets a
# directory of its own rather than sharing one.
#
# It is asked through the shell (`cygpath`) rather than read here as
# $(PROGRAMFILES), and that is load-bearing: make itself may be a 32-bit program,
# and Windows answers a 32-bit process with the x86 directory — which is the
# wrong place to install a 64-bit binary. The shell that answers is the
# environment's own and is not redirected.
#
# The answer is in that shell's spelling, which is what the recipes below open;
# an override is given the same way — `make install BINDIR=/c/tools/sshakku`.
PREFIX ?= $(shell cygpath -u "$$PROGRAMFILES")
BINDIR ?= $(PREFIX)/sshakku
endif
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
ifeq ($(WINDOWS_UNAME),)
USER_BINDIR ?= $(USER_HOME)/.local/bin
else
# The account's own programs, which is where Windows expects a per-user
# application and where an uninstall knows to look. Not under the home directory
# the shell reports: under MSYS that is the environment's home, which need not be
# the account's. Asked of the shell for the same reasons as PREFIX above.
USER_BINDIR ?= $(shell cygpath -u "$$LOCALAPPDATA")/Programs/sshakku
endif
# install-user also puts USER_BINDIR on PATH from the wired login hook (guarded
# so it's a no-op when already present), since it isn't on the default PATH
# everywhere — notably ~/.local/bin on macOS. On by default; set to 0 or empty
# to skip it and wire only the agent hook.
WIRE_PATH ?= 1
# install-user/uninstall-user shell family: "bash" or "zsh". Picks which of
# WIRE_BASHRC/WIRE_ZSHRC gates the non-login rc-file wiring and which
# profile/rc file pair install-user-hook.sh targets; install-user-hook.sh
# itself always prefers an existing .d drop-in directory over the
# marker-block fallback file, whichever shell is selected. Defaults to each
# platform's own login shell: zsh on macOS, bash elsewhere.
ifeq ($(UNAME),Darwin)
USER_SHELL ?= zsh
else
USER_SHELL ?= bash
endif
ifeq ($(USER_SHELL),zsh)
USER_WIRE_RC = $(WIRE_ZSHRC)
else
USER_WIRE_RC = $(WIRE_BASHRC)
endif

GO ?= go
GO_MAIN = ./cmd/sshakku
# Windows runs a program by its extension, so the name it is built under is part
# of whether it can be run at all — and the hook a wiring writes names this path.
ifeq ($(WINDOWS_UNAME),)
GO_BIN = bin/sshakku
else
GO_BIN = bin/sshakku.exe
endif

# The race detector is built on cgo, and the binary this project distributes
# must not need a C toolchain to produce. SSHAKKU_RACE picks between the two:
# set it to any non-empty value and the test targets run under -race with cgo
# available; leave it unset and they run without either, which is the
# configuration the shipped binary is built in. `build` never consults it —
# there is one product binary, and it is always cgo-free.
SSHAKKU_RACE ?=
ifeq ($(SSHAKKU_RACE),)
GO_RACE =
GO_ENV = CGO_ENABLED=0
else
GO_RACE = -race
GO_ENV = CGO_ENABLED=1
endif

# The name ssh is pointed at to have a passphrase prompt answered. It is
# installed as a link to sshakku, which serves that role when it is run under
# this name, so there is one binary and no second program to keep in step. The
# link is made relative, which keeps it valid both inside a DESTDIR staging
# tree and on the system it is eventually unpacked onto.
SSHAKKU_ASKPASS_NAME = sshakku-askpass

ifeq ($(UNAME),Linux)
SSH_INIT_INSTALL_SCRIPT = internal/install/nn-ssh-init.sh
INSTALL_PATH = $(DESTDIR)$(BINDIR)
SSH_INIT_NAME= $(NN)-ssh-init.sh
SSH_INIT_BIND_PATH = $(ETC_PROFILE_D)$(SSH_INIT_NAME)
SSH_INIT_INSTALL_PATH = $(DESTDIR)$(SSH_INIT_BIND_PATH)
SSH_INIT_BASHRC_DROPIN_PATH = $(DESTDIR)$(BASH_BASHRC_D)$(SSH_INIT_NAME)
SSH_INIT_BASHRC_FILE_PATH = $(DESTDIR)$(BASH_BASHRC_FILE)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku
SSHAKKU_ASKPASS_INSTALL_PATH = $(INSTALL_PATH)/$(SSHAKKU_ASKPASS_NAME)
SSHAKKU_RUNTIME_PATH = $(BINDIR)/sshakku

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@install -Dm755 $(GO_BIN) $(SSHAKKU_INSTALL_PATH)
	@echo "Linking $(SSHAKKU_ASKPASS_INSTALL_PATH) to sshakku"
	@ln -sf sshakku $(SSHAKKU_ASKPASS_INSTALL_PATH)
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
	@echo "Uninstalling $(SSHAKKU_ASKPASS_INSTALL_PATH)"
	@rm -f $(SSHAKKU_ASKPASS_INSTALL_PATH)
	@echo "Uninstalling $(SSH_INIT_INSTALL_PATH)"
	@rm -f $(SSH_INIT_INSTALL_PATH)
	@./shell-hook-lib.sh remove-drop-in "$(SSH_INIT_BASHRC_DROPIN_PATH)"
	@./shell-hook-lib.sh strip-block-file "$(SSH_INIT_BASHRC_FILE_PATH)"
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(USER_BINDIR)/sshakku"
	@install -Dm755 $(GO_BIN) $(USER_BINDIR)/sshakku
	@echo "Linking $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME) to sshakku"
	@ln -sf sshakku $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)
	@echo "Wiring the per-user login hook"
	@./install-user-hook.sh install "$(USER_HOME)" "$(USER_BINDIR)/sshakku" "$(NN)" "$(USER_WIRE_RC)" "$(USER_SHELL)" "$(WIRE_PATH)"
	@echo "Installation complete."

uninstall-user:
	@echo "Uninstalling $(USER_BINDIR)/sshakku"
	@rm -f $(USER_BINDIR)/sshakku
	@echo "Uninstalling $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)"
	@rm -f $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)
	@echo "Removing the per-user login hook"
	@./install-user-hook.sh uninstall "$(USER_HOME)" "$(NN)" "$(USER_SHELL)"
	@echo "Uninstallation complete."

else ifeq ($(UNAME),Darwin)
SSH_INIT_INSTALL_SCRIPT = internal/install/nn-ssh-init.sh
INSTALL_PATH = $(DESTDIR)$(BINDIR)
SSH_INIT_NAME = $(NN)-sshakku-init.sh
SSH_INIT_HOOK_RENDERED_PATH = $(DESTDIR)$(SHARE_DIR)$(SSH_INIT_NAME)
SSH_INIT_ZPROFILE_PATH = $(DESTDIR)$(ETC_ZPROFILE)
SSH_INIT_ZSHRC_PATH = $(DESTDIR)$(ETC_ZSHRC)
# print-paths' one shared name for "where the login shell picks this up" —
# on Darwin that's the marker-block file, not the rendered hook itself.
SSH_INIT_INSTALL_PATH = $(SSH_INIT_ZPROFILE_PATH)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku
SSHAKKU_ASKPASS_INSTALL_PATH = $(INSTALL_PATH)/$(SSHAKKU_ASKPASS_NAME)
SSHAKKU_RUNTIME_PATH = $(BINDIR)/sshakku

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@mkdir -p "$(dir $(SSHAKKU_INSTALL_PATH))"
	@install -m755 $(GO_BIN) $(SSHAKKU_INSTALL_PATH)
	@echo "Linking $(SSHAKKU_ASKPASS_INSTALL_PATH) to sshakku"
	@ln -sf sshakku $(SSHAKKU_ASKPASS_INSTALL_PATH)
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
	@echo "Uninstalling $(SSHAKKU_ASKPASS_INSTALL_PATH)"
	@rm -f $(SSHAKKU_ASKPASS_INSTALL_PATH)
	@echo "Removing $(SSH_INIT_HOOK_RENDERED_PATH)"
	@rm -f $(SSH_INIT_HOOK_RENDERED_PATH)
	@rmdir "$(dir $(SSH_INIT_HOOK_RENDERED_PATH))" 2>/dev/null || true
	@./shell-hook-lib.sh strip-block-file "$(SSH_INIT_ZPROFILE_PATH)"
	@./shell-hook-lib.sh strip-block-file "$(SSH_INIT_ZSHRC_PATH)"
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(USER_BINDIR)/sshakku"
	@mkdir -p "$(USER_BINDIR)"
	@install -m755 $(GO_BIN) $(USER_BINDIR)/sshakku
	@echo "Linking $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME) to sshakku"
	@ln -sf sshakku $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)
	@echo "Wiring the per-user login hook"
	@./install-user-hook.sh install "$(USER_HOME)" "$(USER_BINDIR)/sshakku" "$(NN)" "$(USER_WIRE_RC)" "$(USER_SHELL)" "$(WIRE_PATH)"
	@echo "Installation complete."

uninstall-user:
	@echo "Uninstalling $(USER_BINDIR)/sshakku"
	@rm -f $(USER_BINDIR)/sshakku
	@echo "Uninstalling $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)"
	@rm -f $(USER_BINDIR)/$(SSHAKKU_ASKPASS_NAME)
	@echo "Removing the per-user login hook"
	@./install-user-hook.sh uninstall "$(USER_HOME)" "$(NN)" "$(USER_SHELL)"
	@echo "Uninstallation complete."

else ifneq ($(WINDOWS_UNAME),)

# Two jobs here, split by who is able to answer the question. Putting the binary
# where this system keeps programs is this file's, because this file is what
# chose the place. The wiring is the program's own: which file a shell reads is a
# question only that shell can answer, so it is asked rather than assembled from
# paths written down here.
#
# The two meet in one line — the wiring is run from the copy that was just
# installed. The program records its own directory in the stored environment,
# which is how `sshakku` becomes runnable by name in later sessions, and running
# the installed copy is what makes that directory the installed one rather than
# a second answer restated here and able to disagree.
#
# The scopes line up with the other platforms': the plain target installs for the
# machine and needs an elevated prompt, the -user one installs for the account
# and needs nothing. No askpass helper is installed either way — this platform
# has no implementation of one, and a helper that is there but cannot run is
# worse than one that is honestly absent.
#
# With no shell named, the program wires the one the command was typed in, which
# from here is the shell running make. SHELL_ARG passes another: `make
# install-user SHELL_ARG=--shell=windowspowershell`. It reaches the command as
# written, so a value holding a space carries its own quotes:
# `SHELL_ARG='--shell-exe="/c/Program Files/PowerShell/7/pwsh.exe"'`.
SHELL_ARG ?=
# DESTDIR stages an install under a second root, which only works where every
# path hangs off one. Here a path carries the drive it is on, so prefixing it
# builds neither a staged tree nor an error — it builds a directory called
# `stageC:`. Install somewhere else with BINDIR instead, which is a real
# directory and can be run from.
ifneq ($(DESTDIR),)
$(error DESTDIR cannot stage an install on this platform, where a path names its own drive; \
	choose the directory with BINDIR or USER_BINDIR instead)
endif
INSTALL_PATH = $(BINDIR)
USER_INSTALL_PATH = $(USER_BINDIR)
SSHAKKU_INSTALL_PATH = $(INSTALL_PATH)/sshakku.exe
SSHAKKU_USER_INSTALL_PATH = $(USER_INSTALL_PATH)/sshakku.exe
SSHAKKU_RUNTIME_PATH = $(SSHAKKU_INSTALL_PATH)
SSHAKKU_ASKPASS_INSTALL_PATH =

install: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_INSTALL_PATH)"
	@mkdir -p "$(INSTALL_PATH)"
	@cp -f "$(GO_BIN)" "$(SSHAKKU_INSTALL_PATH)"
	"$(SSHAKKU_INSTALL_PATH)" install --scope=machine $(SHELL_ARG)
	@echo "Installation complete."

# The installed copy is the one asked to take the wiring out, because it is the
# one whose directory was recorded and the only thing that knows it. Where there
# is none — nothing was installed, or an earlier version of this file wired the
# binary where it was built — the built one answers instead and takes its own
# directory back out, which is the one that would have been recorded then.
uninstall: build
	@installed="$(SSHAKKU_INSTALL_PATH)"; [ -x "$$installed" ] || installed="$(GO_BIN)"; \
		echo "Removing the wiring, with $$installed"; \
		"$$installed" uninstall --scope=machine $(SHELL_ARG)
	@echo "Uninstalling $(SSHAKKU_INSTALL_PATH)"
	@rm -f "$(SSHAKKU_INSTALL_PATH)"
	@rmdir "$(INSTALL_PATH)" 2>/dev/null || true
	@echo "Uninstallation complete."

install-user: build
	@echo "Installing $(GO_BIN) to $(SSHAKKU_USER_INSTALL_PATH)"
	@mkdir -p "$(USER_INSTALL_PATH)"
	@cp -f "$(GO_BIN)" "$(SSHAKKU_USER_INSTALL_PATH)"
	"$(SSHAKKU_USER_INSTALL_PATH)" install --scope=user $(SHELL_ARG)
	@echo "Installation complete."

uninstall-user: build
	@installed="$(SSHAKKU_USER_INSTALL_PATH)"; [ -x "$$installed" ] || installed="$(GO_BIN)"; \
		echo "Removing the wiring, with $$installed"; \
		"$$installed" uninstall --scope=user $(SHELL_ARG)
	@echo "Uninstalling $(SSHAKKU_USER_INSTALL_PATH)"
	@rm -f "$(SSHAKKU_USER_INSTALL_PATH)"
	@rmdir "$(USER_INSTALL_PATH)" 2>/dev/null || true
	@echo "Uninstallation complete."

else

install uninstall install-user uninstall-user:
	@echo "$(UNAME) is not supported."
	@exit 1
endif

build:
	CGO_ENABLED=0 $(GO) build -o $(GO_BIN) $(GO_MAIN)

# Every platform this tree has source for, built from wherever this runs and
# without a C compiler or an SDK for any of them. It answers one question — can
# this be built where it is not run — which is what a release has to be able to
# do, and it is also the only way one platform's own source gets compiled from
# another machine. Windows is here as a build, not as a supported target: what
# it holds is the code that reports what this system does not do, and it is
# listed so that stops compiling loudly rather than quietly.
build-cross:
	CGO_ENABLED=0 GOOS=darwin $(GO) build ./...
	CGO_ENABLED=0 GOOS=linux $(GO) build ./...
	CGO_ENABLED=0 GOOS=windows $(GO) build ./...

test:
	$(GO_ENV) $(GO) test $(GO_RACE) ./...

# CI-only variant of test: same run under gotestsum (nicer condensed CI
# output; requires it on PATH, unlike plain `test`), capturing the same
# `go test -json` event stream (--jsonfile) and a coverage profile for
# tools/testreport and gopogh to summarize. gotestsum exits with go test's
# own status, so `make test-json` still fails the build on a test failure
# like plain `make test` does. go-ignore-cov then marks blocks tagged
# `//coverage:ignore` as covered, so genuinely untestable code (e.g. main's
# lone os.Exit) doesn't drag the reported percentage down; it warns and no-ops
# if it can't resolve packages (e.g. an unsatisfied local toolchain), never
# failing the build.
test-json:
	$(GO_ENV) gotestsum --jsonfile test.json -- $(GO_RACE) -coverprofile=coverage.out ./...
	go-ignore-cov --file coverage.out --root .

# Goroutine leak checks. The normal suite already runs every package under
# go.uber.org/goleak, so a goroutine outliving its package's tests fails the
# build. This target additionally rebuilds under Go's experimental goroutine
# leak profiler (GOEXPERIMENT=goroutineleakprofile) and runs the *GoroutineLeak*
# tests, which use the runtime's reachability analysis to catch a goroutine
# blocked forever — a class the running-count check cannot see. Those tests
# skip when the profiler isn't compiled in, so they are inert in `make test`.
test-leakprofile:
	GOEXPERIMENT=goroutineleakprofile $(GO) test -run GoroutineLeak ./...

# Live macOS keychain integration test: the real Add/Find/Update/Delete/List
# round trip through DarwinKeychainClient against the process's default
# keychain. Only meaningful on macOS, and only once the default keychain is one
# the caller is content to write to — CI repoints it at a throwaway keychain
# first (test/macos-keychain-setup.sh). The test skips unless
# SSHAKKU_TEST_ALLOW_REAL_KEYCHAIN is set; -count=1 defeats go's build cache,
# which has no way to see the keychain's external state.
#
# Pinned to CGO_ENABLED=0 rather than following SSHAKKU_RACE: this is the only
# test that reaches the framework for real, so it is the only one that can catch
# a wrong signature in the keychain client — and it has to catch it in the
# configuration the shipped binary is built in.
test-keychain:
	SSHAKKU_TEST_ALLOW_REAL_KEYCHAIN=1 CGO_ENABLED=0 $(GO) test -count=1 -run TestDarwinKeychainClientRealRoundTrip ./internal/keys/...

# Shell-level login-hook and agent-lifecycle regression suite. Requires
# bats-core; only safe in a disposable environment (the container test suite
# runs it in CI) — see test/bats/helpers.bash for the explicit opt-in gate.
test-bats:
	bats test/bats

# What a platform has no such file for is reported as absent rather than as a
# blank, which reads like a setting that failed to expand.
NOT_HERE = (nothing of this kind is installed on this platform)

print-paths:
	@echo "PREFIX: $(PREFIX)"
	@echo "BINDIR: $(BINDIR)"
	@echo "DESTDIR: $(DESTDIR)"
	@echo "SSHAKKU_INSTALL_PATH: $(SSHAKKU_INSTALL_PATH)"
	@echo "SSHAKKU_ASKPASS_INSTALL_PATH: $(if $(SSHAKKU_ASKPASS_INSTALL_PATH),$(SSHAKKU_ASKPASS_INSTALL_PATH),$(NOT_HERE))"
	@echo "SSHAKKU_RUNTIME_PATH: $(SSHAKKU_RUNTIME_PATH)"
	@echo "SSH_INIT_INSTALL_PATH: $(if $(SSH_INIT_INSTALL_PATH),$(SSH_INIT_INSTALL_PATH),$(NOT_HERE))"
	@echo "USER_HOME: $(USER_HOME)"
	@echo "USER_BINDIR: $(USER_BINDIR)"
	@echo "USER_SHELL: $(USER_SHELL)"

# Linting. Requires: shellcheck, shfmt, markdownlint-cli2, taplo, checkmake,
# actionlint, editorconfig-checker, hadolint, blinter, zsh, pwsh with the
# PSScriptAnalyzer module. Each tool reads its own config file where it has one.
# The bats fixtures are a mixed bag: executable stand-ins for real tools, which
# are shell, alongside config files a test drops in to select a backend. Only
# the former belong to shellcheck; the rest are linted by the tool for their own
# format (config files by taplo, via lint-toml).
BATS_FIXTURES = $(filter-out %.toml,$(wildcard test/bats/fixtures/*))
SH_SCRIPTS = $(wildcard *.sh) $(wildcard .githooks/*) $(wildcard .github/scripts/*.sh) $(wildcard test/*.sh) $(wildcard test/containers/*.sh) $(wildcard test/fakes/*.sh) $(wildcard test/bats/*.bats) $(wildcard test/bats/*.bash) $(shell find cmd internal -name "*.sh") $(BATS_FIXTURES)
ZSH_SCRIPTS = $(wildcard *.zsh)
# Found rather than globbed at a fixed depth: a fixture that moves with the
# package it belongs to must not stop being linted without anything saying so.
BAT_FILES = $(shell find cmd internal -path "*/testdata/*.cmd")
DOCKERFILES = $(wildcard test/containers/*.Dockerfile)

# Found rather than globbed at a fixed depth: a script that moves to another
# package must not stop being linted without anything saying so.
APPLESCRIPTS = $(shell find internal -name "*.applescript")
XML_FILES = $(wildcard internal/*/testdata/*.xml)

# The login hook lives at the top level beside its Bourne counterpart; the rest
# are found rather than globbed at a fixed depth, for the same reason as above.
PS1_FILES = $(wildcard *.ps1) $(shell find cmd internal tools -name "*.ps1")

lint: lint-sh lint-zsh lint-bat lint-md lint-toml lint-make lint-yaml lint-editorconfig lint-go lint-docker lint-applescript lint-ps1 lint-xml

lint-sh:
	shellcheck $(SH_SCRIPTS)
	shfmt -d $(SH_SCRIPTS)

# zsh has no shellcheck/shfmt-equivalent linter; -n gives a real but
# syntax-only check (no style/portability warnings).
lint-zsh:
	@for f in $(ZSH_SCRIPTS); do zsh -n "$$f" || exit 1; done

# Windows batch. One file at a time, so the one that failed is named by the
# line above its output rather than inferred from it.
lint-bat:
	@for f in $(BAT_FILES); do echo "blinter $$f"; blinter "$$f" || exit 1; done

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

# golangci-lint analyses one build and one only: the host's GOOS with no build
# tags. Files behind another platform's tag, or behind a failure-injection tag,
# are not skipped with a note — they are never looked at. Every build this
# project compiles therefore has to be named here, whether or not it is one
# the project ships or runs its tests on. `golangci-lint fmt`
# needs no such list: it reads the files, not the build.
lint-go:
	golangci-lint fmt --diff
	CGO_ENABLED=0 GOOS=linux $(GO) vet ./...
	CGO_ENABLED=0 GOOS=darwin $(GO) vet ./...
	CGO_ENABLED=0 GOOS=windows $(GO) vet ./...
	CGO_ENABLED=0 GOOS=linux golangci-lint run
	CGO_ENABLED=0 GOOS=darwin golangci-lint run
	CGO_ENABLED=0 GOOS=windows golangci-lint run
	CGO_ENABLED=0 GOOS=linux golangci-lint run --build-tags=backend_unresponsive
	CGO_ENABLED=0 GOOS=linux golangci-lint run --build-tags=midsession_failure

lint-docker:
	hadolint $(DOCKERFILES)

# osacompile is the only AppleScript checker there is, and it ships only with
# macOS — so on any other system this reports that it did not run rather than
# passing quietly, which would read the same as having checked.
lint-applescript:
	@if ! command -v osacompile >/dev/null 2>&1; then \
		echo "skipping lint-applescript: osacompile is macOS-only and is not installed here"; \
	else \
		for f in $(APPLESCRIPTS); do echo "osacompile $$f"; osacompile -o /dev/null "$$f" || exit 1; done; \
	fi

# PSScriptAnalyzer is the PowerShell linter, and it is a PowerShell module: it
# needs a pwsh to run in. On a machine without one this reports that it did not
# run rather than passing quietly, which would read the same as having checked.
lint-ps1:
	@if ! command -v pwsh >/dev/null 2>&1; then \
		echo "skipping lint-ps1: PSScriptAnalyzer runs in pwsh, which is not installed here"; \
	else \
		pwsh -NoProfile -File tools/lint-ps1.ps1 $(PS1_FILES); \
	fi

# Well-formedness only: the DTD these files name lives at an http:// URL, and
# xmllint is not asked to fetch it, so the check stays offline.
lint-xml:
	xmllint --noout $(XML_FILES)

.PHONY: install uninstall install-user uninstall-user build build-cross test test-json test-leakprofile test-keychain test-bats print-paths lint lint-sh lint-zsh lint-bat lint-md lint-toml lint-make lint-yaml lint-editorconfig lint-go lint-docker lint-applescript lint-ps1 lint-xml
.DEFAULT_GOAL := install
