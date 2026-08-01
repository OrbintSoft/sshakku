package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

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

	// The wallet the operating system provides itself has no case of its own
	// here: it is this platform's default, and is covered by the per-platform
	// tests beside newDefaultSecretBackend.
}

// TestBitwardenMasterPrompterGUI covers the graphical branch of the wallet
// master-password prompter: given a dialog, it delegates to it and returns the
// answer trimmed. Available is unconditionally true.
//
// The dialog is a stand-in rather than the platform's own, because which dialog
// a platform draws with is not what this test judges — that it is used when
// there is one, and that the terminal is not reached for, is.
func TestBitwardenMasterPrompterGUI(t *testing.T) {
	p := walletPasswordPrompter{graphical: fixedPrompter{answer: "master-pass"}}

	got, err := p.Prompt("Bitwarden master password")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "master-pass" {
		t.Errorf("Prompt = %q, want %q, the answer the dialog gave", got, "master-pass")
	}
	if !p.Available() {
		t.Error("Available() = false, want true")
	}
}

// fixedPrompter is a dialog that always answers, standing in for whichever one
// the platform would draw with.
type fixedPrompter struct{ answer string }

func (p fixedPrompter) Prompt(string) (string, error) { return p.answer, nil }
func (fixedPrompter) Available() bool                 { return true }
