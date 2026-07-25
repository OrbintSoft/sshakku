package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// fakeRunner returns a canned result for any command, so a prompter can be
// exercised without launching a real dialog binary.
type fakeRunner struct {
	res keys.Result
	err error
}

func (f fakeRunner) Run(keys.Cmd) (keys.Result, error) { return f.res, f.err }

// TestNewSecretBackend covers the CLI-backed backend branches, whose
// construction is pure (no D-Bus, no subprocess): the returned value has the
// expected concrete type and fields, and the cleanup func is always callable.
// The default secret-service branch is left to integration coverage — it dials
// the session bus.
func TestNewSecretBackend(t *testing.T) {
	// Keep the bitwarden branch's prompter probe headless and subprocess-free.
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	t.Run("1password", func(t *testing.T) {
		s := config.Settings{SecretBackend: config.SecretBackendOnePassword, OnePasswordVault: "sshakku-vault"}
		backend, closeFn := newSecretBackend("alice", fakeLogger{}, s)
		defer closeFn()
		op, ok := backend.(*keys.OnePasswordBackend)
		if !ok {
			t.Fatalf("backend = %T, want *keys.OnePasswordBackend", backend)
		}
		if op.Vault != "sshakku-vault" {
			t.Errorf("Vault = %q, want %q", op.Vault, "sshakku-vault")
		}
	})

	t.Run("bitwarden", func(t *testing.T) {
		s := config.Settings{
			SecretBackend:   config.SecretBackendBitwarden,
			BitwardenEmail:  "alice@example.com",
			BitwardenServer: "https://vault.example",
		}
		backend, closeFn := newSecretBackend("alice", fakeLogger{}, s)
		defer closeFn()
		bw, ok := backend.(*keys.BitwardenBackend)
		if !ok {
			t.Fatalf("backend = %T, want *keys.BitwardenBackend", backend)
		}
		if bw.Email != "alice@example.com" || bw.Server != "https://vault.example" {
			t.Errorf("Email/Server = %q/%q, want alice@example.com/https://vault.example", bw.Email, bw.Server)
		}
		if bw.Prompter == nil {
			t.Error("Prompter = nil, want a bitwarden master prompter")
		}
	})

	t.Run("keychain", func(t *testing.T) {
		s := config.Settings{SecretBackend: config.SecretBackendKeychain}
		backend, closeFn := newSecretBackend("alice", fakeLogger{}, s)
		defer closeFn()
		if backend == nil {
			t.Fatal("backend = nil, want a keychain backend")
		}
	})
}

// TestBitwardenMasterPrompterGUI covers the graphical branch of the bitwarden
// master-password prompter: with gui set it delegates to kdialog, returning its
// trimmed stdout. Available is unconditionally true.
func TestBitwardenMasterPrompterGUI(t *testing.T) {
	runner := fakeRunner{res: keys.Result{Stdout: []byte("master-pass\n"), Code: 0}}
	p := bitwardenMasterPrompter{kdialog: keys.KDialogPrompter{Runner: runner}, gui: true}

	got, err := p.Prompt("Bitwarden master password")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "master-pass" {
		t.Errorf("Prompt = %q, want %q (kdialog stdout, trimmed)", got, "master-pass")
	}
	if !p.Available() {
		t.Error("Available() = false, want true")
	}
}
