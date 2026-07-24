//go:build unix

package keys

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	// defaultAddTimeout caps each ssh-add so a stuck prompt cannot hang login.
	defaultAddTimeout = 60 * time.Second
	// defaultKeyTTL bounds how long the stashed passphrase lives in the
	// handoff (keyring entry or socket, depending on OS), so an entry
	// ssh-add never reads still expires.
	defaultKeyTTL = 60 * time.Second
)

// ExecKeyAdder adds keys with the real ssh-add.
type ExecKeyAdder struct {
	// AskpassProg is the absolute path to the SSH_ASKPASS helper — the sshakku
	// binary under a name that runs `askpass`. Required by AddWithAskpass.
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

// AddWithAskpass stashes passphrase in the OS-appropriate handoff (see
// handoff_linux.go/handoff_darwin.go), then runs ssh-add detached from any
// terminal so it fetches the passphrase through the SSH_ASKPASS helper keyed
// by the handoff token. The passphrase never enters argv or the inherited
// environment of any other process.
func (a ExecKeyAdder) AddWithAskpass(keyfile, passphrase string) (int, error) {
	ttl := a.KeyTTL
	if ttl == 0 {
		ttl = defaultKeyTTL
	}
	token, err := stashPassphrase(passphrase, ttl)
	if err != nil {
		return 0, fmt.Errorf("stash passphrase: %w", err)
	}

	env := []string{
		"SSH_ASKPASS=" + a.AskpassProg,
		"SSH_ASKPASS_REQUIRE=force",
		EnvAskpassMode + "=1",
		EnvPassHandoffToken + "=" + token,
	}
	env = passThrough(env, "PATH", "HOME", "USER", "DISPLAY", "WAYLAND_DISPLAY",
		"SSH_AUTH_SOCK", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "DBUS_SESSION_BUS_ADDRESS")
	return a.runSSHAdd(env, keyfile)
}

// runSSHAdd runs `ssh-add <keyfile>` with env, detached from any terminal
// (setsid, no stdin) so it fetches the passphrase via SSH_ASKPASS and its own
// chatter is discarded, returning its exit code.
func (a ExecKeyAdder) runSSHAdd(env []string, keyfile string) (int, error) {
	to := a.AddTimeout
	if to == 0 {
		to = defaultAddTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh-add", sshAddArgs(a.KeyLifetime, keyfile)...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
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
