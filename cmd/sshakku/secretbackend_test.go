package main

import (
	"context"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		backend, closeFn := newSecretBackend(t.Context(), "alice", fakeLogger{}, s)
		defer closeFn()
		op, ok := backend.(*wallet.OnePassword)
		require.Truef(t, ok, "the wallet chosen must be the one built, got %T", backend)
		assert.Equal(t, "sshakku-vault", op.Vault, "the vault the configuration named")
	})

	t.Run("bitwarden", func(t *testing.T) {
		s := config.Settings{
			SecretBackend:   config.SecretBackendBitwarden,
			BitwardenEmail:  "alice@example.com",
			BitwardenServer: "https://vault.example",
		}
		backend, closeFn := newSecretBackend(t.Context(), "alice", fakeLogger{}, s)
		defer closeFn()
		bw, ok := backend.(*wallet.Bitwarden)
		require.Truef(t, ok, "the wallet chosen must be the one built, got %T", backend)
		assert.Equal(t, "alice@example.com", bw.Email, "the account the configuration named")
		assert.Equal(t, "https://vault.example", bw.Server, "the server the configuration named")
		assert.NotNil(t, bw.Prompter, "a wallet that needs a master password must have something to ask with")
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

	got, err := p.Prompt(t.Context(), "Bitwarden master password")
	require.NoError(t, err, "Prompt")
	assert.Equal(t, "master-pass", got, "the answer the dialog gave, with nothing added to it")
	assert.True(t, p.Available(t.Context()), "a prompter with a dialog behind it is always available")
}

// fixedPrompter is a dialog that always answers, standing in for whichever one
// the platform would draw with.
type fixedPrompter struct{ answer string }

func (p fixedPrompter) Prompt(context.Context, string) (string, error) { return p.answer, nil }
func (fixedPrompter) Available(context.Context) bool                   { return true }
