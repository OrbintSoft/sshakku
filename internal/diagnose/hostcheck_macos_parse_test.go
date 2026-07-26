package diagnose

import "testing"

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
			got := parseFileVaultStatus([]byte(c.out))
			switch {
			case c.want == nil && got != nil:
				t.Errorf("parseFileVaultStatus(%q) = %v, want nil", c.out, *got)
			case c.want != nil && (got == nil || *got != *c.want):
				t.Errorf("parseFileVaultStatus(%q) = %v, want %v", c.out, got, *c.want)
			}
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
			if got == nil {
				t.Fatalf("bridgeSecureEnclave(%q) = nil, want a definite %v", c.out, c.want)
			}
			if *got != c.want || kind != c.wantKind {
				t.Errorf("bridgeSecureEnclave(%q) = (%v, %q), want (%v, %q)", c.out, *got, kind, c.want, c.wantKind)
			}
		})
	}
}

func boolp(b bool) *bool { return &b }
