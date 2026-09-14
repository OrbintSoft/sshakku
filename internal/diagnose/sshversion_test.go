package diagnose

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/sshtools"
	"github.com/stretchr/testify/assert"
)

// Verifies F63. What is being asked of the report is not "is this version
// old" — it is whether a user on the older side of the line is told, since
// there is nothing else that would tell them: the wiring is printed as usual,
// the exports are set as usual, and the only thing that happens differently is
// that the wallet is never reached.

// tooOld and newEnough are real answers from real builds: the one Microsoft
// shipped in Windows for years, and the one this was written against.
var (
	tooOld    = sshtools.Version{Major: 8, Minor: 1, Text: "OpenSSH_8.1p1, OpenSSL 1.0.2s  28 May 2019"}
	newEnough = sshtools.Version{Major: 9, Minor: 5, Text: "OpenSSH_for_Windows_9.5p2, LibreSSL 3.8.2"}
)

// TestGatherNamesABuildTooOldToBeSentToAHelper. Below 8.4 there is no way to
// tell ssh to use a helper it would not have used on its own, and on a system
// that names no X display it would not have: the wallet is then unreachable
// from that shell no matter how correctly everything else is wired.
func TestGatherNamesABuildTooOldToBeSentToAHelper(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		SSHVersion: tooOld,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "8.1"),
		"the report has to name the build that was found: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "8.4"),
		"and the one it would take, since the reader's next move is to compare the two: %v", r.Findings)
}

// TestGatherSaysNothingAboutABuildThatIsNewEnough: every ordinary machine,
// where a line about a version would be in every report anybody ever pasted
// into a bug report.
func TestGatherSaysNothingAboutABuildThatIsNewEnough(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		SSHVersion: newEnough,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "8.4"),
		"a build that can be told to ask is not worth a line: %v", r.Findings)
}

// TestGatherClaimsNothingAboutAVersionNobodyRead. A version that could not be
// read is not an old one, and the zero value must not become the finding for
// the oldest build imaginable — which is exactly what a numeric comparison
// against an unfilled struct would make it.
func TestGatherClaimsNothingAboutAVersionNobodyRead(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "8.4"),
		"nobody read a version, so nothing may be said about one: %v", r.Findings)
}

// TestGatherSaysBothWhereBothAreTrue. A shell with no exports and a build that
// could not have used them are two separate facts, and the reader needs both:
// the wiring finding sends them to open a login shell, which here will produce
// the same report again, and the version finding is the half that says why.
// Neither may swallow the other.
func TestGatherSaysBothWhereBothAreTrue(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		SSHVersion: tooOld,
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "8.4"),
		"the build has to be named: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"and the unwired shell still is, since it is separately true: %v", r.Findings)
}
