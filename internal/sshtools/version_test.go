package sshtools

import (
	"context"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F63. The strings below are what real builds print, not shapes
// invented here: OpenSSH answers `-V` on standard error, gives the Windows
// build a name of its own, and puts the portable release after a `p` that is
// not part of the version this is comparing.

// versionSayer is an OpenSSH that answers `-V` with one fixed line, which is
// all this question needs from one. It stands in for the program, never for
// the deciding: what the line means is the thing under test.
type versionSayer struct {
	stderr string
	stdout string
	err    error
	code   int
	args   []string
}

func (s *versionSayer) Run(_ context.Context, c run.Cmd) (run.Result, error) {
	s.args = append([]string{c.Name}, c.Args...)
	return run.Result{Stdout: []byte(s.stdout), Stderr: []byte(s.stderr), Code: s.code}, s.err
}

// TestTheVersionIsReadOutOfWhatTheBuildPrints covers the spellings a reader of
// this project will actually meet, on both sides of the release that matters.
func TestTheVersionIsReadOutOfWhatTheBuildPrints(t *testing.T) {
	for _, tc := range []struct {
		name         string
		printed      string
		major, minor int
	}{
		{"the build this was written against", "OpenSSH_for_Windows_9.5p2, LibreSSL 3.8.2", 9, 5},
		{"an ordinary portable build", "OpenSSH_9.6p1, OpenSSL 3.0.13 30 Jan 2024", 9, 6},
		{"the one Windows shipped for years", "OpenSSH_8.1p1, OpenSSL 1.0.2s  28 May 2019", 8, 1},
		{"a long-lived enterprise distribution", "OpenSSH_7.4p1, OpenSSL 1.0.2k-fips  26 Jan 2017", 7, 4},
		{"a release with no portable suffix at all", "OpenSSH_9.9, LibreSSL 4.0.0", 9, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := ParseVersion(tc.printed)
			require.NoErrorf(t, err, "a line a build really prints has to be readable: %q", tc.printed)
			assert.Equal(t, tc.major, v.Major, "major")
			assert.Equal(t, tc.minor, v.Minor, "minor")
			assert.Equal(t, tc.printed, v.Text,
				"the report names what the build said, so what it said is kept whole")
		})
	}
}

// TestTheBuildIsNamedWithoutTheLibraryItWasLinkedAgainst. What OpenSSH prints
// for `-V` is two facts in one line, and only the first is being compared. A
// report that quotes the whole of it puts a second comma in a sentence that
// already has one, and offers a crypto library's release date to a reader
// deciding whether to upgrade ssh.
func TestTheBuildIsNamedWithoutTheLibraryItWasLinkedAgainst(t *testing.T) {
	for printed, want := range map[string]string{
		"OpenSSH_for_Windows_9.5p2, LibreSSL 3.8.2":       "OpenSSH_for_Windows_9.5p2",
		"OpenSSH_8.1p1, OpenSSL 1.0.2s  28 May 2019":      "OpenSSH_8.1p1",
		"OpenSSH_7.4p1, OpenSSL 1.0.2k-fips  26 Jan 2017": "OpenSSH_7.4p1",
		"OpenSSH_9.3p1 Ubuntu-3ubuntu3.6, OpenSSL 3.0.10": "OpenSSH_9.3p1 Ubuntu-3ubuntu3.6",
	} {
		v, err := ParseVersion(printed)
		require.NoError(t, err, printed)
		assert.Equal(t, want, v.Label(), printed)
	}
}

// TestAVersionThatCannotBeReadIsNotGuessedAt. The cost of guessing is not a
// wrong number in a report: it is telling somebody with a current OpenSSH that
// their build is too old, which sends them to replace the one thing that was
// never the problem.
func TestAVersionThatCannotBeReadIsNotGuessedAt(t *testing.T) {
	for _, printed := range []string{
		"",
		"usage: ssh [-46AaCfGgKkMNnqsTtVvXxYy]",
		"OpenSSH_",
		"Dropbear v2022.83",
		"OpenSSH_for_Windows_, LibreSSL 3.8.2",
		// Digits in the right place and still not a release: a number no
		// release could be is refused rather than wrapped into one that looks
		// plausible.
		"OpenSSH_99999999999999999999.1p1, LibreSSL 3.8.2",
		"OpenSSH_9.99999999999999999999p1, LibreSSL 3.8.2",
	} {
		v, err := ParseVersion(printed)
		require.Errorf(t, err, "an answer that is not a version must be refused: %q", printed)
		assert.Equal(t, Version{}, v,
			"and refusing has to leave nothing behind, since a half-filled version reads as a very old one")
	}
}

// TestABuildTooOldToBeSentToAHelperIsToldApartFromOneThatIsNot holds the one
// number this rests on. SSH_ASKPASS_REQUIRE arrived in OpenSSH 8.4; below it
// there is no way to ask for a helper that the build would not have reached on
// its own, and on a system naming no X display it would not have reached one.
func TestABuildTooOldToBeSentToAHelperIsToldApartFromOneThatIsNot(t *testing.T) {
	for _, tc := range []struct {
		v          Version
		canBeAsked bool
	}{
		{Version{Major: 8, Minor: 3}, false},
		{Version{Major: 8, Minor: 4}, true},
		{Version{Major: 8, Minor: 10}, true},
		{Version{Major: 7, Minor: 9}, false},
		{Version{Major: 9, Minor: 0}, true},
		{Version{Major: 10, Minor: 0}, true},
	} {
		assert.Equalf(t, tc.canBeAsked, tc.v.CanBeToldToAsk(),
			"OpenSSH %d.%d", tc.v.Major, tc.v.Minor)
	}
}

// TestAVersionNobodyReadIsNotAnOldOne. The zero value has to be the third
// answer rather than the smallest number, or every report that could not run
// `ssh -V` would carry the finding for the oldest build there has ever been.
func TestAVersionNobodyReadIsNotAnOldOne(t *testing.T) {
	assert.True(t, Version{}.Unread(), "the zero value is nobody having looked")
	assert.False(t, Version{Major: 9, Minor: 5}.Unread(), "and a version that was read is not that")
	assert.True(t, Version{}.CanBeToldToAsk(),
		"and it answers the question the way silence does, since the alternative is telling "+
			"somebody with a current OpenSSH to replace the one thing that was never the problem")
}

// TestAProgramThatWillNotSayWhatItIsLeavesNoVersionBehind. A build that cannot
// be started at all is not an old build, and the error has to come back as one
// rather than as a version of zero.
func TestAProgramThatWillNotSayWhatItIsLeavesNoVersionBehind(t *testing.T) {
	v, err := ReadVersion(t.Context(), &versionSayer{err: errNotOnPath}, "ssh")
	require.Error(t, err)
	assert.True(t, v.Unread(), "and nothing is left behind that a comparison could read")
}

// TestAVersionPrintedOnStandardOutputIsStillRead. Where OpenSSH answers is
// settled; where a wrapper or a repackaged build answers is not, and one that
// prints the same line on the other stream is telling us the same thing.
func TestAVersionPrintedOnStandardOutputIsStillRead(t *testing.T) {
	v, err := ReadVersion(t.Context(), &versionSayer{stdout: "OpenSSH_9.6p1, OpenSSL 3.0.13\n"}, "ssh")
	require.NoError(t, err)
	assert.Equal(t, 9, v.Major)
	assert.Equal(t, 6, v.Minor)
}

// TestTheVersionIsAskedOfTheProgramOnStandardError. OpenSSH prints `-V` to
// standard error, so a reader of stdout alone finds an empty string and calls
// every build unreadable.
func TestTheVersionIsAskedOfTheProgramOnStandardError(t *testing.T) {
	said := &versionSayer{stderr: "OpenSSH_9.5p2, LibreSSL 3.8.2\n"}

	v, err := ReadVersion(t.Context(), said, "/usr/bin/ssh")
	require.NoError(t, err)
	assert.Equal(t, 9, v.Major)
	assert.Equal(t, []string{"/usr/bin/ssh", "-V"}, said.args,
		"and it is asked of the program named, with the flag that answers")
}
