//go:build unix

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTamperedEnvVarsHandledSafely drives the real environment-reading paths
// with hostile or malformed values in the variables SSHakku trusts from the
// ambient shell, and asserts each is caught rather than obeyed. Unlike the
// unit tests that inject faked inputs, these read the actual process
// environment through the same os.Getenv calls production uses, run the real
// Gather against the real /proc and sockets, and redeem a handoff token
// through the real store — so what is exercised is the tampering defence as it
// ships, not a stand-in for it.
func TestTamperedEnvVarsHandledSafely(t *testing.T) {
	// A hijacked SSH_AUTH_SOCK pointing at a socket that does not answer must
	// be reported as unreachable, never silently trusted as the live agent.
	t.Run("SSH_AUTH_SOCK points at a dead socket", func(t *testing.T) {
		tempRuntimeEnv(t)
		bogus := filepath.Join(t.TempDir(), "not-an-agent.sock")
		t.Setenv("SSH_AUTH_SOCK", bogus)

		env := paths.FromOS()
		report := gatherReport(t.Context(), env, paths.Resolve(env, paths.ProbeDir), config.Settings{})

		assert.Equal(t, bogus, report.EnvSock, "the report must show the value the shell really carries")
		assert.False(t, report.EnvReachable, "a socket nothing is listening on is not a live agent")
		assert.Truef(t, findingContains(report.Findings, "not answering"),
			"and the user must be told: %q", report.Findings)
	})

	// A cleared SSH_AUTH_SOCK (another way to derail a shell) must surface as
	// "no agent reachable", not pass unnoticed.
	t.Run("SSH_AUTH_SOCK unset", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv("SSH_AUTH_SOCK", "")

		env := paths.FromOS()
		report := gatherReport(t.Context(), env, paths.Resolve(env, paths.ProbeDir), config.Settings{})

		assert.Empty(t, report.EnvSock, "a shell that exported nothing has nothing to show")
		assert.Truef(t, findingContains(report.Findings, "SSH_AUTH_SOCK is unset"),
			"and a cleared variable must not pass unnoticed: %q", report.Findings)
	})

	// SSHAKKU_ASKPASS is set in the login shells of installations that wired
	// themselves in before SSHakku stopped reading it, and an environment
	// SSHakku does not read must not be able to change what it does: a command
	// typed in such a shell dispatches as itself.
	t.Run("a leftover SSHAKKU_ASKPASS marker changes nothing", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv("SSHAKKU_ASKPASS", "1")

		var stdout, stderr bytes.Buffer
		code := dispatch(t.Context(), realDeps(), &stdout, &stderr, "/usr/local/bin/sshakku", []string{"help"})

		require.Zerof(t, code, "asking for help is not a failure (stderr: %s)", stderr.String())
		assert.Contains(t, stdout.String(), "usage: sshakku",
			"a variable SSHakku does not read must not be able to change what it does")
	})

	// A tampered SSHAKKU_HANDOFF_TOKEN must be rejected by the real store and
	// yield no passphrase on stdout — a malformed handle can never redeem a
	// stash that was never made under it.
	t.Run("malformed SSHAKKU_HANDOFF_TOKEN yields no passphrase", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv(keys.EnvPassHandoffToken, "not-a-real-token")

		var stdout bytes.Buffer
		code := realDeps().askpass(t.Context(), &stdout, []string{"Enter passphrase:"})

		assert.Equal(t, 1, code, "a handle no stash was made under can redeem nothing")
		assert.Empty(t, stdout.String(), "and a tampered token must leak nothing at all")
	})
}

// findingContains reports whether any finding mentions substr.
func findingContains(findings []string, substr string) bool {
	for _, f := range findings {
		if strings.Contains(f, substr) {
			return true
		}
	}
	return false
}
