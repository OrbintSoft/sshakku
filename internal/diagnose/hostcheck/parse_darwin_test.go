//go:build darwin

package hostcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFileVaultStatus covers every branch of the fdesetup-status parser:
// a definite On, a definite Off, and unrecognized output that yields nil.
func TestParseFileVaultStatus(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want *bool
	}{
		{"on", "FileVault is On.\n", boolp(true)},
		{"off", "FileVault is Off.\n", boolp(false)},
		{"deferred/unknown", "FileVault is On (Deferred enable pending).\n", boolp(true)},
		{"unrecognized", "some other tool output", nil},
		{"empty", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A nil answer and a false one are different things — "the output
			// said nothing I understand" and "FileVault is off" — and Equal on
			// the pointers keeps them apart.
			assert.Equal(t, c.want, parseFileVaultStatus([]byte(c.out)),
				"what this fdesetup output says about FileVault")
		})
	}
}

// TestBridgeSecureEnclave covers the T1/T2 detection: a definite present for
// either chip, and a definite absent (never nil) with an empty kind otherwise.
func TestBridgeSecureEnclave(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		want     bool
		wantKind string
	}{
		{"T2 present", "Apple T2 Security Chip\n", true, "Secure Enclave"},
		{"T1 present", "  Model Name: Apple T1 Security Chip", true, "Secure Enclave"},
		{"absent", "iBridge: not available", false, ""},
		{"empty", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, kind := bridgeSecureEnclave([]byte(c.out))
			require.NotNil(t, got, "output that was read settles the question either way")
			assert.Equal(t, c.want, *got, "whether this machine carries a security chip")
			assert.Equal(t, c.wantKind, kind, "what the report calls it")
		})
	}
}

func boolp(b bool) *bool { return &b }
