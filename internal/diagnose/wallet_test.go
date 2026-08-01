package diagnose

import (
	"bytes"
	"strings"
	"testing"
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
	if len(missing) != 1 || missing[0].Name != "database" {
		t.Fatalf("Missing() = %+v, want only the database", missing)
	}
	if len(WalletView{}.Missing()) != 0 {
		t.Error("a wallet with no requirements has nothing missing")
	}
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
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want one", findings)
	}
	for _, want := range []string{"bitwarden", "bw", "not on PATH"} {
		if !strings.Contains(findings[0], want) {
			t.Errorf("finding %q does not mention %q", findings[0], want)
		}
	}
	if WalletFindings(view.withEverythingPresent()) != nil {
		t.Error("a wallet with nothing missing must produce no findings")
	}
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
		name    string
		wallet  WalletView
		want    []string
		absent  []string
		heading bool
	}{
		{
			name:    "a backend with a route names both",
			wallet:  WalletView{Backend: "keepassxc", Route: "cli", Requirements: []Requirement{{Name: "keepassxc-cli", Detail: "nowhere"}}},
			want:    []string{"wallet:", "keepassxc", "route: cli", "keepassxc-cli:", "missing", "nowhere"},
			heading: true,
		},
		{
			name:    "a backend with one way to be reached names no route",
			wallet:  WalletView{Backend: "keychain"},
			want:    []string{"wallet:", "keychain"},
			absent:  []string{"route:"},
			heading: true,
		},
		{
			name:    "a satisfied requirement is not called missing",
			wallet:  WalletView{Backend: "1password", Requirements: []Requirement{{Name: "op", Detail: "/usr/bin/op", Present: true}}},
			want:    []string{"op:", "found", "/usr/bin/op"},
			absent:  []string{"missing"},
			heading: true,
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
				if !strings.Contains(got, want) {
					t.Errorf("report does not contain %q:\n%s", want, got)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Errorf("report should not contain %q:\n%s", absent, got)
				}
			}
		})
	}
}
