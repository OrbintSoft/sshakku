//go:build linux

package keys

import (
	"errors"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// saveKeyringHandoffSeams snapshots the keyring-op seams (and the shared RNG)
// used by the Linux passphrase handoff, restoring them when the (sub)test ends.
func saveKeyringHandoffSeams(t *testing.T) {
	t.Helper()
	oRand := randRead
	oAdd, oRead, oUnlink, oTimeout := keyringAdd, keyringRead, keyringUnlink, keyringSetTimeout
	t.Cleanup(func() {
		randRead = oRand
		keyringAdd, keyringRead, keyringUnlink, keyringSetTimeout = oAdd, oRead, oUnlink, oTimeout
	})
}

func TestStashPassphrase(t *testing.T) {
	t.Run("token RNG fails", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
		if _, err := stashPassphrase("secret", time.Minute); err == nil {
			t.Fatal("stashPassphrase returned nil error, want the RNG failure")
		}
	})

	t.Run("keyring add fails", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 0, errors.New("EDQUOT") }
		if _, err := stashPassphrase("secret", time.Minute); err == nil {
			t.Fatal("stashPassphrase returned nil error, want the keyring-add failure")
		}
	})

	t.Run("success returns the serial as a token and sets a timeout", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		var gotTTL time.Duration
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 4242, nil }
		keyringSetTimeout = func(_ keyring.Serial, d time.Duration) error { gotTTL = d; return nil }
		token, err := stashPassphrase("secret", 90*time.Second)
		if err != nil {
			t.Fatalf("stashPassphrase: %v", err)
		}
		if token != "4242" {
			t.Fatalf("token = %q, want \"4242\"", token)
		}
		if gotTTL != 90*time.Second {
			t.Fatalf("timeout set = %v, want 90s", gotTTL)
		}
	})
}

func TestFetchPassphrase(t *testing.T) {
	t.Run("malformed token", func(t *testing.T) {
		if _, err := fetchPassphrase("not-a-serial"); err == nil {
			t.Fatal("fetchPassphrase returned nil error, want a malformed-token error")
		}
	})

	t.Run("keyring read fails, key still unlinked", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		unlinked := false
		keyringRead = func(keyring.Serial) ([]byte, error) { return nil, errors.New("EKEYEXPIRED") }
		keyringUnlink = func(keyring.Serial) error { unlinked = true; return nil }
		if _, err := fetchPassphrase("7"); err == nil {
			t.Fatal("fetchPassphrase returned nil error, want the read failure")
		}
		if !unlinked {
			t.Fatal("fetchPassphrase did not unlink the key after a failed read")
		}
	})

	t.Run("round trip reads then unlinks the key", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		var unlinked keyring.Serial
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("s3cr3t"), nil }
		keyringUnlink = func(s keyring.Serial) error { unlinked = s; return nil }
		got, err := fetchPassphrase("7")
		if err != nil {
			t.Fatalf("fetchPassphrase: %v", err)
		}
		if got != "s3cr3t" {
			t.Fatalf("fetchPassphrase = %q, want %q", got, "s3cr3t")
		}
		if unlinked != 7 {
			t.Fatalf("unlinked serial = %d, want 7", unlinked)
		}
	})
}
