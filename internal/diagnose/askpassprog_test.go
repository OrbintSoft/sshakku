package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// there and notThere are the two answers a caller that went and looked can give
// about the askpass helper. A nil is the third: nobody looked.
var (
	there    = true
	notThere = false
)

// TestGatherNamesTheAskpassHelperThatIsNotThere. Where the helper is gone the
// exports are deliberately not printed, so the shell has no SSH_ASKPASS — which
// on its own reads as a shell nobody wired. The two have the same symptom and
// opposite remedies: one is put right by opening a login shell, and the other
// by nothing a login shell can do.
func TestGatherNamesTheAskpassHelperThatIsNotThere(t *testing.T) {
	r := Gather(t.Context(), Inputs{ //nolint:gosec // G101 matches AskpassProg; the value is a path to a program
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		AskpassProg: "/usr/bin/sshakku-askpass", AskpassProgThere: &notThere,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "sshakku-askpass"),
		"the report has to name the program that is not there: %v", r.Findings)
	assert.Falsef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"and must not also give the remedy for the other cause of the same symptom: %v", r.Findings)
}

// TestGatherDoesNotSendAnyoneToALoginShellForAMissingHelper. The one thing this
// finding must never say is what every other unwired-shell finding says: no
// session can wire an export pointing at a program that is not there, so a
// reader who opens one comes back to the same report.
func TestGatherDoesNotSendAnyoneToALoginShellForAMissingHelper(t *testing.T) {
	r := Gather(t.Context(), Inputs{ //nolint:gosec // G101 matches AskpassProg; the value is a path to a program
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		AskpassProg: "/usr/bin/sshakku-askpass", AskpassProgThere: &notThere,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	for _, f := range r.Findings {
		for _, sendsThemAway := range []string{"log out", "new session", "new login shell"} {
			assert.NotContainsf(t, f, sendsThemAway,
				"a helper that is not there is not put back by opening a shell: %q", f)
		}
	}
}

// TestGatherSaysNothingAboutAHelperThatIsThere: the ordinary machine, where
// this finding would be noise in every report ever pasted into a bug.
func TestGatherSaysNothingAboutAHelperThatIsThere(t *testing.T) {
	r := Gather(t.Context(), Inputs{ //nolint:gosec // G101 matches EnvAskpass; the value is a path to this program
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		EnvAskpass: "/usr/bin/sshakku-askpass", EnvAskpassRequire: "force",
		AskpassProg: "/usr/bin/sshakku-askpass", AskpassProgThere: &there,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "is not beside"),
		"a helper that is there must not be reported missing: %v", r.Findings)
}

// TestGatherClaimsNothingAboutAHelperNobodyLookedFor. A report that was not
// able to look is not evidence of absence — the same rule the environment
// findings follow when they are reporting on somebody else's session.
func TestGatherClaimsNothingAboutAHelperNobodyLookedFor(t *testing.T) {
	r := Gather(t.Context(), Inputs{ //nolint:gosec // G101 matches AskpassProg; the value is a path to a program
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		AskpassProg: "/usr/bin/sshakku-askpass",
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "is not beside"),
		"nobody looked, so nothing may be claimed: %v", r.Findings)
}
