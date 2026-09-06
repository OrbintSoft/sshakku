package keys

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// namedSSHAdd is a program named by its full path rather than by the name a
// session would look up, which is what a caller hands down on a system where
// the two are not the same program.
var namedSSHAdd = filepath.Join("some", "where", "ssh-add")

// naming answers with prog, the way a session that had to choose between builds
// of OpenSSH does.
func naming(prog string) SSHAddNamer { return func() (string, error) { return prog, nil } }

// errNoBuildReachesTheAgent is a session whose only OpenSSH cannot reach the
// agent it was pointed at.
var errNoBuildReachesTheAgent = errors.New("no build of ssh-add here can reach this system's agent")

// refusing is the answer a session with no usable ssh-add gets.
func refusing() SSHAddNamer {
	return func() (string, error) { return "", errNoBuildReachesTheAgent }
}

// addingWith runs one key through the adder with the running intercepted, and
// hands back the program it was about to run. Only the spawning is replaced;
// which program the adder chose is decided before that and is not stood in for.
func addingWith(t *testing.T, adder ExecKeyAdder) (string, error) {
	t.Helper()

	oldStash, oldRun := stashPass, runCmd
	t.Cleanup(func() { stashPass, runCmd = oldStash, oldRun })

	var ran string
	stashPass = func(string, time.Duration) (string, error) { return "a-token", nil }
	runCmd = func(cmd *exec.Cmd) error {
		ran = cmd.Args[0]
		return nil
	}

	_, err := adder.AddWithAskpass(t.Context(), "id_ed25519", "hunter2")
	return ran, err
}

// TestTheKeyAdderRunsTheProgramItWasNamed. Where a system has more than one
// build of OpenSSH, which one is run is settled once and every key goes to that
// one — not to whatever the session's PATH resolves at the moment a key is
// loaded, which on such a system is the build the shell brought with it.
func TestTheKeyAdderRunsTheProgramItWasNamed(t *testing.T) {
	ran, err := addingWith(t, ExecKeyAdder{AskpassProg: "sshakku", SSHAdd: naming(namedSSHAdd)})

	require.NoError(t, err)
	assert.Equal(t, namedSSHAdd, ran,
		"a passphrase handed to a build that reaches no agent comes back as though it were wrong")
}

// TestTheKeyAdderFallsBackToTheOrdinaryName, so a caller with nothing to say
// about which build to run gets the session's own — the answer everywhere that
// has only one.
func TestTheKeyAdderFallsBackToTheOrdinaryName(t *testing.T) {
	ran, err := addingWith(t, ExecKeyAdder{AskpassProg: "sshakku"})

	require.NoError(t, err)
	assert.Equal(t, "ssh-add", ran)
}

// TestNoPassphraseIsStashedForAKeyThatCannotBeAdded. The passphrase is put
// somewhere ssh-add's helper can come and fetch it; where no ssh-add is going
// to be started, nothing will come, and the answer is to not put it there
// rather than to leave it waiting out its lifetime.
func TestNoPassphraseIsStashedForAKeyThatCannotBeAdded(t *testing.T) {
	oldStash, oldRun := stashPass, runCmd
	t.Cleanup(func() { stashPass, runCmd = oldStash, oldRun })

	stashed, started := 0, 0
	stashPass = func(string, time.Duration) (string, error) {
		stashed++
		return "a-token", nil
	}
	runCmd = func(*exec.Cmd) error {
		started++
		return nil
	}

	_, err := ExecKeyAdder{AskpassProg: "sshakku", SSHAdd: refusing()}.
		AddWithAskpass(t.Context(), "id_ed25519", "hunter2")

	require.ErrorIs(t, err, errNoBuildReachesTheAgent, "the caller has to be told why the key was not added")
	assert.Zero(t, stashed, "the passphrase was put where nothing was ever going to fetch it")
	assert.Zero(t, started, "and something was started that could not have worked")
}

// TestTheAgentIsReadThroughTheProgramNamed: the snapshot of what the agent
// already holds decides which keys are loaded at all, and a build that cannot
// reach the agent reports it empty — which is indistinguishable from an agent
// that really is.
func TestTheAgentIsReadThroughTheProgramNamed(t *testing.T) {
	r := runtest.NewRunner().On(namedSSHAdd, runtest.Stdout("256 SHA256:abc one (ED25519)\n", 0))

	got, err := AgentFingerprints(t.Context(), r, namedSSHAdd)

	require.NoError(t, err)
	assert.True(t, got["SHA256:abc"], "the listing came back, so the program named was the one asked")
	require.Len(t, r.Calls, 1)
	assert.Equal(t, namedSSHAdd, r.Calls[0].Name)
}

// TestTheAgentIsReadThroughTheOrdinaryNameWhereNoneWasGiven.
func TestTheAgentIsReadThroughTheOrdinaryNameWhereNoneWasGiven(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 1))

	_, err := AgentFingerprints(t.Context(), r, "")

	require.NoError(t, err)
	require.Len(t, r.Calls, 1)
	assert.Equal(t, "ssh-add", r.Calls[0].Name)
}

// TestNoKeyIsLoadedThroughABuildThatReachesNoAgent. The loader stops at the
// snapshot rather than going on to ask for passphrases: every answer would be
// read as wrong, and the keys would be given up on one after another.
func TestNoKeyIsLoadedThroughABuildThatReachesNoAgent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("key"), 0o600))
	r := runtest.NewRunner()

	err := Loader{
		Keys:   Enumerator{Dir: dir},
		Runner: r,
		SSHAdd: refusing(),
	}.LoadKeys(t.Context())

	require.ErrorIs(t, err, errNoBuildReachesTheAgent)
	assert.Empty(t, r.Calls, "nothing may be run through a program that cannot reach the agent")
}

// TestAnAccountWithNoKeysIsToldNothingAboutSSHAdd. Where there is no key to
// load there is no ssh-add to run, so a session that has none is not a session
// with a problem — and saying so would put an error in front of somebody with
// nothing to fix.
func TestAnAccountWithNoKeysIsToldNothingAboutSSHAdd(t *testing.T) {
	err := Loader{
		Keys:   Enumerator{Dir: filepath.Join(t.TempDir(), "no-ssh-directory-here")},
		Runner: runtest.NewRunner(),
		SSHAdd: refusing(),
	}.LoadKeys(t.Context())

	require.NoError(t, err)
}

// TestAnExpiredKeyIsTakenOutThroughTheProgramNamed. A removal sent to a build
// that reaches no agent comes back as "the agent does not have it", and the
// record is dropped on that answer — so the key stays in the agent past its
// lifetime with nothing left to say it should not be.
func TestAnExpiredKeyIsTakenOutThroughTheProgramNamed(t *testing.T) {
	r := runtest.NewRunner().On(namedSSHAdd, runtest.Stdout("", 0))
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   filepath.Join("keys", "id_rsa"),
		AddedAt:   expiryClock.Add(-2 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}

	expirer := expirerWith(records, r, nil)
	expirer.SSHAdd = naming(namedSSHAdd)
	require.NoError(t, expirer.ExpireKeys(t.Context()))

	require.Len(t, r.Calls, 1)
	assert.Equal(t, namedSSHAdd, r.Calls[0].Name)
}

// TestAKeyIsLeftInTheAgentWhenThereIsNoWayToTakeItOut, together with its
// record: nothing was asked of the agent, so the next session has to try again
// rather than forget a key that is still in there.
func TestAKeyIsLeftInTheAgentWhenThereIsNoWayToTakeItOut(t *testing.T) {
	r := runtest.NewRunner()
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   filepath.Join("keys", "id_rsa"),
		AddedAt:   expiryClock.Add(-2 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}

	expirer := expirerWith(records, r, nil)
	expirer.SSHAdd = refusing()
	require.NoError(t, expirer.ExpireKeys(t.Context()), "a session that cannot expire a key still opens")

	assert.Empty(t, r.Calls, "nothing may be run through a program that cannot reach the agent")
	assert.Empty(t, records.forgotten, "the record is what makes the next session try again")
}
