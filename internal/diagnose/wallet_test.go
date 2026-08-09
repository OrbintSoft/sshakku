package diagnose

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletMissingKeepsOnlyWhatIsNotThere(t *testing.T) {
	view := WalletView{
		Backend: "keepassxc",
		Requirements: []Requirement{
			{Name: "keepassxc-cli", Detail: "/usr/bin/keepassxc-cli", Present: true},
			{Name: "database", Detail: "not configured"},
		},
	}
	missing := view.Missing()
	require.Len(t, missing, 1, "only the unmet requirement is missing")
	assert.Equal(t, "database", missing[0].Name, "the requirement that was not met")
	assert.Empty(t, WalletView{}.Missing(), "a wallet with no requirements has nothing missing")
}

func TestWalletFindingsNameTheWalletAndThePiece(t *testing.T) {
	view := WalletView{
		Backend: "bitwarden",
		Requirements: []Requirement{
			{Name: "bw", Detail: "not on PATH"},
			{Name: "session bus", Detail: "/run/bus", Present: true},
		},
	}
	findings := WalletFindings(view)
	require.Len(t, findings, 1, "one unmet requirement, one finding")
	// Three things a reader needs to act on it: which wallet, which piece, and
	// what is wrong with it. Assert, so one run names every one left out.
	assert.Contains(t, findings[0], "bitwarden", "the finding must name the wallet")
	assert.Contains(t, findings[0], "bw", "the finding must name the piece that is missing")
	assert.Contains(t, findings[0], "not on PATH", "the finding must say what is wrong with it")

	assert.Nil(t, WalletFindings(view.withEverythingPresent()), "a wallet with nothing missing must produce no findings")
}

// withEverythingPresent returns the same wallet with every requirement met, so
// the "nothing to report" case is built from the same fixture rather than from
// a second one that might drift from it.
func (w WalletView) withEverythingPresent() WalletView {
	out := WalletView{Backend: w.Backend, Route: w.Route}
	for _, req := range w.Requirements {
		req.Present = true
		out.Requirements = append(out.Requirements, req)
	}
	return out
}

func TestFormatWalletSection(t *testing.T) {
	tests := []struct {
		name   string
		wallet WalletView
		want   []string
		absent []string
	}{
		{
			name:   "a backend with a route names both",
			wallet: WalletView{Backend: "keepassxc", Route: "cli", Requirements: []Requirement{{Name: "keepassxc-cli", Detail: "nowhere"}}},
			want:   []string{"wallet:", "keepassxc", "route: cli", "keepassxc-cli:", "missing", "nowhere"},
		},
		{
			name:   "a backend with one way to be reached names no route",
			wallet: WalletView{Backend: "keychain"},
			want:   []string{"wallet:", "keychain"},
			absent: []string{"route:"},
		},
		{
			name:   "a satisfied requirement is not called missing",
			wallet: WalletView{Backend: "1password", Requirements: []Requirement{{Name: "op", Detail: "/usr/bin/op", Present: true}}},
			want:   []string{"op:", "found", "/usr/bin/op"},
			absent: []string{"missing"},
		},
		{
			name: "a requirement nobody could settle is neither found nor missing",
			wallet: WalletView{Backend: "secret-service", Requirements: []Requirement{
				{Name: "compartment", Detail: "no wallet was answering to ask", Undetermined: true},
			}},
			want:   []string{"compartment:", "undetermined", "no wallet was answering"},
			absent: []string{"missing", "found"},
		},
		{
			name:   "no backend resolved prints no section at all",
			wallet: WalletView{},
			absent: []string{"wallet:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			Format(&out, Report{Wallet: tc.wallet})
			got := out.String()
			for _, want := range tc.want {
				assert.Containsf(t, got, want, "the wallet section must say %q", want)
			}
			for _, absent := range tc.absent {
				assert.NotContainsf(t, got, absent, "the wallet section must not say %q", absent)
			}
		})
	}
}
