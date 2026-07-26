//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
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
		report := gatherReport(env, paths.Resolve(env, paths.ProbeDir))

		if report.EnvSock != bogus {
			t.Errorf("report.EnvSock = %q, want the tampered value %q", report.EnvSock, bogus)
		}
		if report.EnvReachable {
			t.Error("report.EnvReachable = true for a socket nothing is listening on")
		}
		if !findingContains(report.Findings, "not answering") {
			t.Errorf("no finding flagged the unreachable SSH_AUTH_SOCK; findings = %q", report.Findings)
		}
	})

	// A cleared SSH_AUTH_SOCK (another way to derail a shell) must surface as
	// "no agent reachable", not pass unnoticed.
	t.Run("SSH_AUTH_SOCK unset", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv("SSH_AUTH_SOCK", "")

		env := paths.FromOS()
		report := gatherReport(env, paths.Resolve(env, paths.ProbeDir))

		if report.EnvSock != "" {
			t.Errorf("report.EnvSock = %q, want empty", report.EnvSock)
		}
		if !findingContains(report.Findings, "SSH_AUTH_SOCK is unset") {
			t.Errorf("no finding flagged the missing SSH_AUTH_SOCK; findings = %q", report.Findings)
		}
	})

	// SSHAKKU_ASKPASS lingers in every login shell once askpass-env runs, so a
	// subcommand typed by hand afterwards must still dispatch as itself and
	// never be hijacked into the SSH_ASKPASS broker by the leftover marker.
	t.Run("SSHAKKU_ASKPASS does not hijack a real subcommand", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv(keys.EnvAskpassMode, "1")

		var stdout, stderr bytes.Buffer
		askpassEnvSet := os.Getenv(keys.EnvAskpassMode) != ""
		code := dispatch(realDeps(), &stdout, &stderr, []string{"help"}, askpassEnvSet)

		if code != 0 {
			t.Fatalf("dispatch(help) exit = %d, want 0 (stderr: %s)", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "usage: sshakku") {
			t.Errorf("help output did not run; the askpass marker hijacked dispatch. stdout: %s", stdout.String())
		}
	})

	// A tampered SSHAKKU_HANDOFF_TOKEN must be rejected by the real store and
	// yield no passphrase on stdout — a malformed handle can never redeem a
	// stash that was never made under it.
	t.Run("malformed SSHAKKU_HANDOFF_TOKEN yields no passphrase", func(t *testing.T) {
		tempRuntimeEnv(t)
		t.Setenv(keys.EnvPassHandoffToken, "not-a-real-token")

		var stdout bytes.Buffer
		code := realDeps().askpass(&stdout, []string{"Enter passphrase:"})

		if code != 1 {
			t.Errorf("askpass with a bogus handoff token exit = %d, want 1", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("askpass emitted %q on stdout; a tampered token must leak nothing", stdout.String())
		}
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
