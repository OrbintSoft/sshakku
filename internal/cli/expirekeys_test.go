package cli

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// F53: only a system whose agent expires nothing has anything to do here, and
// only where there is an agent to take a key out of. Both answers are checked
// from either machine, since which one a system gives is the system's own.
func TestExpiringIsOnlyThisSessionsJobWhereTheAgentKeepsNoLifetimes(t *testing.T) {
	assert.True(t, expiryIsThisSessionsJob(false, `\\.\pipe\openssh-ssh-agent`),
		"an agent that holds no lifetimes leaves the keeping of them to the session")
	assert.False(t, expiryIsThisSessionsJob(true, "/run/sshakku/agent.sock"),
		"an agent that holds lifetimes has already dropped the key at its own deadline")
	assert.False(t, expiryIsThisSessionsJob(false, ""),
		"with no agent there is nothing to take a key out of")
}

func TestARecordIsReadAsWhenTheKeyWasAddedAndWhenItRunsOut(t *testing.T) {
	dir := t.TempDir()
	keyfile := filepath.Join(t.TempDir(), "keys", "id_rsa")
	added := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	store := keystate.Store{Dir: dir, Now: func() time.Time { return added }}
	require.NoError(t, store.Save(keyfile, time.Hour), "Save")

	records := keyRecords{store: store}
	got, err := records.Added()
	require.NoError(t, err, "Added")
	require.Len(t, got, 1, "one key was recorded")
	assert.Equal(t, keyfile, got[0].KeyFile, "the file the key was added from")
	assert.Truef(t, got[0].AddedAt.Equal(added), "AddedAt = %v, want %v", got[0].AddedAt, added)
	assert.Truef(t, got[0].ExpiresAt.Equal(added.Add(time.Hour)),
		"ExpiresAt = %v, want %v", got[0].ExpiresAt, added.Add(time.Hour))

	require.NoError(t, records.Forget(keyfile), "Forget")
	got, err = records.Added()
	require.NoError(t, err, "Added after Forget")
	assert.Empty(t, got, "a forgotten key is no longer one the store says was added")
}

func TestAKeyAddedWithNoLifetimeIsReadAsNeverRunningOut(t *testing.T) {
	store := keystate.Store{Dir: t.TempDir()}
	require.NoError(t, store.Save(filepath.Join("keys", "id_rsa"), 0), "Save")

	got, err := keyRecords{store: store}.Added()
	require.NoError(t, err, "Added")
	require.Len(t, got, 1, "one key was recorded")
	assert.True(t, got[0].ExpiresAt.IsZero(), "no lifetime was asked for, so there is no moment it runs out")
}

// aShellOpeningOver seeds a key added nine hours ago to live one hour, opens a
// shell against an agent that either keeps lifetimes or does not, and hands back
// what ssh-add was asked to do and the store as it was left.
func aShellOpeningOver(t *testing.T, agentKeepsLifetimes bool) (*runtest.Runner, keystate.Store, string) {
	t.Helper()

	tempRuntimeEnv(t)
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir).WithSocketToken(paths.SocketToken())
	keyfile := filepath.Join(t.TempDir(), "keys", "id_rsa")
	store := keystate.Store{
		Dir: keystateDir(layout),
		Now: func() time.Time { return time.Now().Add(-9 * time.Hour) },
	}
	require.NoError(t, store.Save(keyfile, time.Hour), "seed a key added nine hours ago for one hour")

	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))
	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	d.runner = r
	d.agentKeepsLifetimes = agentKeepsLifetimes

	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil),
		"shellInit must succeed; stderr=%q", errOut.String())
	return r, store, keyfile
}

// F53: the session takes the key out — so a shell opening is what has to do it,
// not a command a person would have to remember to run. Which half runs is
// decided by what this system's agent does with lifetimes, and both answers are
// driven here rather than only the one this machine gives: the other is somebody
// else's every login.
func TestAShellOpeningTakesOutAKeyWhoseTimeIsUp(t *testing.T) {
	t.Run("an agent that keeps no lifetimes leaves the keeping of them to the session", func(t *testing.T) {
		r, store, keyfile := aShellOpeningOver(t, false)

		require.Len(t, r.Calls, 1, "the key whose time is up must be taken out as the shell opens")
		assert.Equal(t, "ssh-add", r.Calls[0].Name, "the agent is reached through the program that speaks to it")
		assert.Equal(t, []string{"-d", keyfile}, r.Calls[0].Args, "and the key is named by the file it was added from")
		_, found := store.Load("id_rsa")
		assert.False(t, found, "the record goes with the key, or every later session tries again")
	})

	t.Run("an agent that keeps lifetimes has already dropped it", func(t *testing.T) {
		r, store, _ := aShellOpeningOver(t, true)

		assert.Empty(t, r.Calls,
			"this agent expires keys itself, so a session that removed one would be removing what somebody re-added")
		_, found := store.Load("id_rsa")
		assert.True(t, found, "and the record stays, since nothing here has taken the key away")
	})
}

func TestAShellOpeningLeavesAKeyStillWithinItsTime(t *testing.T) {
	tempRuntimeEnv(t)
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir).WithSocketToken(paths.SocketToken())
	store := keystate.Store{Dir: keystateDir(layout)}
	require.NoError(t, store.Save(filepath.Join(t.TempDir(), "id_rsa"), 8*time.Hour), "seed a key added just now")

	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))
	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	d.runner = r

	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil),
		"shellInit must succeed; stderr=%q", errOut.String())

	assert.Empty(t, r.Calls, "a key whose lifetime has not elapsed must survive a shell opening")
	_, found := store.Load("id_rsa")
	assert.True(t, found, "and so must its record")
}
