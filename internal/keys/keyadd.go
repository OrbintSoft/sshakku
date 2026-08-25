package keys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys/handoff"
)

const (
	// defaultAddTimeout caps each ssh-add so a stuck prompt cannot hang login.
	defaultAddTimeout = 60 * time.Second
	// defaultKeyTTL bounds how long the stashed passphrase lives in the
	// handoff (keyring entry or socket, depending on OS), so an entry
	// ssh-add never reads still expires.
	defaultKeyTTL = 60 * time.Second
)

// Seams over the passphrase stash and the ssh-add invocation, so
// AddWithAskpass's stash-failure branch and runSSHAdd's exit-code handling are
// exercisable without a live keyring or a real ssh-add. Production points them
// at the real stash and os/exec run.
var (
	stashPass = handoff.Stash
	runCmd    = func(cmd *exec.Cmd) error {
		// Running ssh-add is an external-process side effect; runSSHAdd's exit-code
		// and start-failure handling around this call is unit-tested by stubbing runCmd.
		//coverage:ignore
		return cmd.Run()
	}
)

// ExecKeyAdder adds keys with the real ssh-add.
type ExecKeyAdder struct {
	// AskpassProg is the absolute path to the SSH_ASKPASS helper, which ssh-add
	// execs with the prompt as its one argument and reads the answer from its
	// standard output. Required by AddWithAskpass.
	AskpassProg string
	// AddTimeout caps each ssh-add; 0 uses defaultAddTimeout.
	AddTimeout time.Duration
	// KeyTTL bounds the stashed passphrase's lifetime; 0 uses defaultKeyTTL.
	KeyTTL time.Duration
	// KeyLifetime caps how long the added key stays in the agent (ssh-add -t),
	// after which it must be re-added from the vault. 0 adds the key with no
	// expiry; the caller resolves the default.
	KeyLifetime time.Duration
}

// AddWithAskpass stashes passphrase in the handoff this system provides, then
// runs ssh-add detached from any terminal so it fetches the passphrase through
// the SSH_ASKPASS helper keyed by the handoff token. The passphrase never
// enters argv or the inherited environment of any other process.
func (a ExecKeyAdder) AddWithAskpass(ctx context.Context, keyfile, passphrase string) (int, error) {
	ttl := a.KeyTTL
	if ttl == 0 {
		ttl = defaultKeyTTL
	}
	token, err := stashPass(passphrase, ttl)
	if err != nil {
		return 0, fmt.Errorf("stash passphrase: %w", err)
	}

	env := []string{
		"SSH_ASKPASS=" + a.AskpassProg,
		"SSH_ASKPASS_REQUIRE=force",
		handoff.EnvToken + "=" + token,
	}
	env = passThrough(env, childEnvNames(platformChildEnv)...)
	return a.runSSHAdd(ctx, env, keyfile)
}

// runSSHAdd runs `ssh-add <keyfile>` with env, detached from any terminal (see
// detachFromTerminal) and with no stdin, so it fetches the passphrase via
// SSH_ASKPASS and its own chatter is discarded, returning its exit code.
func (a ExecKeyAdder) runSSHAdd(ctx context.Context, env []string, keyfile string) (int, error) {
	to := a.AddTimeout
	if to == 0 {
		to = defaultAddTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh-add", sshAddArgs(a.KeyLifetime, keyfile)...)
	cmd.Env = env
	detachFromTerminal(cmd)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	err := runCmd(cmd)
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return ee.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// sshAddArgs builds the ssh-add argument list, prepending "-t <seconds>" when a
// positive lifetime caps how long the key stays in the agent before it expires.
// A sub-second lifetime would round to "-t 0" (immediate expiry), so it is
// treated as no expiry instead.
func sshAddArgs(lifetime time.Duration, keyfile string) []string {
	if secs := int64(lifetime / time.Second); secs > 0 {
		return []string{"-t", strconv.FormatInt(secs, 10), keyfile}
	}
	return []string{keyfile}
}

// childEnvNames is every variable ssh-add and the askpass helper it starts are
// given out of this process's environment: the three that mean the same thing
// everywhere, and then whatever this system names for itself.
//
// The system's half arrives as an argument rather than being read here, so the
// composing stays one piece of logic that either machine's tests can check
// against either machine's answer (see platformChildEnv on each).
//
// PATH is how ssh-add is found and how it finds the helper; HOME is the account
// whose keys and settings are in question, which both of them read; and
// SSH_AUTH_SOCK is the agent this session settled on, which is not always the
// one a bare ssh-add would pick for itself.
func childEnvNames(platform []string) []string {
	names := []string{"PATH", "HOME", "SSH_AUTH_SOCK"}
	return append(names, platform...)
}

// passThrough appends "NAME=value" for each named variable that is set, leaving
// the child env minimal — only what ssh-add and the askpass helper need.
func passThrough(env []string, names ...string) []string {
	for _, name := range names {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

var _ KeyAdder = ExecKeyAdder{}
