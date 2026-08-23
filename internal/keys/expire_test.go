package keys

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// fakeAddedKeys is an in-memory AddedKeys: it answers Added from a scripted
// list and records what was forgotten.
type fakeAddedKeys struct {
	keys      []AddedKey
	err       error
	forgetErr error
	forgotten []string
}

func (f *fakeAddedKeys) Added() ([]AddedKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.keys, nil
}

func (f *fakeAddedKeys) Forget(keyfile string) error {
	if f.forgetErr != nil {
		return f.forgetErr
	}
	f.forgotten = append(f.forgotten, keyfile)
	return nil
}

// expiryClock is a fixed instant the tests place records around.
var expiryClock = time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)

func expirerWith(records *fakeAddedKeys, r run.Runner, log Logger) Expirer {
	return Expirer{Records: records, Runner: r, Log: log, Now: func() time.Time { return expiryClock }}
}

func TestAKeyPastItsLifetimeIsTakenOutOfTheAgent(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   "/ssh/id_rsa",
		AddedAt:   expiryClock.Add(-9 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))
	log := &fakeLogger{}

	require.NoError(t, expirerWith(records, r, log).ExpireKeys(t.Context()), "ExpireKeys")

	require.Len(t, r.Calls, 1, "the one key past its time must be the one command run")
	assert.Equal(t, []string{"-d", "/ssh/id_rsa"}, r.Calls[0].Args,
		"the key is named to ssh-add by the file it was added from")
	assert.Equal(t, []string{"/ssh/id_rsa"}, records.forgotten,
		"a key taken out of the agent leaves no record behind, or every later session tries again")
	assert.True(t, log.contains("took id_rsa out of the agent"),
		"the session log must name what was taken out; lines were: %v", log.lines)
}

func TestAKeyStillWithinItsLifetimeIsLeftAlone(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   "/ssh/id_rsa",
		AddedAt:   expiryClock.Add(-time.Hour),
		ExpiresAt: expiryClock.Add(time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))

	require.NoError(t, expirerWith(records, r, &fakeLogger{}).ExpireKeys(t.Context()), "ExpireKeys")

	assert.Empty(t, r.Calls, "a key whose time has not come must not be touched")
	assert.Empty(t, records.forgotten, "nor its record")
}

func TestAKeyAddedWithNoExpiryIsNeverTakenOut(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile: "/ssh/id_rsa",
		AddedAt: expiryClock.Add(-100 * 24 * time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))

	require.NoError(t, expirerWith(records, r, &fakeLogger{}).ExpireKeys(t.Context()), "ExpireKeys")

	assert.Empty(t, r.Calls, "no lifetime was ever asked for, so there is no moment at which it runs out")
}

func TestARecordThatDoesNotSayWhichFileIsNotActedOn(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		AddedAt:   expiryClock.Add(-9 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))

	require.NoError(t, expirerWith(records, r, &fakeLogger{}).ExpireKeys(t.Context()), "ExpireKeys")

	assert.Empty(t, r.Calls, "there is no file to name to ssh-add, and guessing one would remove somebody else's key")
	assert.Empty(t, records.forgotten, "and nothing was done that the record could stop saying")
}

func TestAnAgentThatHasNotGotTheKeyStillEndsTheRecord(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   "/ssh/id_rsa",
		AddedAt:   expiryClock.Add(-9 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 1))
	log := &fakeLogger{}

	require.NoError(t, expirerWith(records, r, log).ExpireKeys(t.Context()), "ExpireKeys")

	assert.Equal(t, []string{"/ssh/id_rsa"}, records.forgotten,
		"a removal that cannot succeed will not succeed next time either, so the record has said all it can")
	assert.True(t, log.contains("the agent does not have id_rsa"),
		"and the reason is written down rather than passed over; lines were: %v", log.lines)
}

func TestSSHAddThatWillNotStartLeavesTheRecordForTheNextSession(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   "/ssh/id_rsa",
		AddedAt:   expiryClock.Add(-9 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Fails(errors.New("exec: no such file")))
	log := &fakeLogger{}

	require.NoError(t, expirerWith(records, r, log).ExpireKeys(t.Context()),
		"a session must open whether or not ssh-add could be run")

	assert.Empty(t, records.forgotten,
		"nothing was asked of the agent, so the key is still there and the record still has work to do")
	assert.True(t, log.contains("could not run ssh-add to take out id_rsa"),
		"and the failure is written down; lines were: %v", log.lines)
}

func TestEveryExpiredKeyIsTriedEvenAfterOneCannotBeRemoved(t *testing.T) {
	past := AddedKey{AddedAt: expiryClock.Add(-9 * time.Hour), ExpiresAt: expiryClock.Add(-time.Hour)}
	first, second := past, past
	first.KeyFile = "/ssh/id_first"
	second.KeyFile = "/ssh/id_second"
	records := &fakeAddedKeys{keys: []AddedKey{first, second}}
	r := &runtest.Recorder{Errs: []error{errors.New("exec: no such file")}}

	require.NoError(t, expirerWith(records, r, &fakeLogger{}).ExpireKeys(t.Context()), "ExpireKeys")

	require.Len(t, r.Calls, 2, "one key that could not be removed must not end the others' expiry")
	assert.Equal(t, []string{"-d", "/ssh/id_second"}, r.Calls[1].Args, "the second key is still tried")
}

func TestTheAgentSSHAddIsPointedAtIsTheOneThisSessionHas(t *testing.T) {
	records := &fakeAddedKeys{keys: []AddedKey{{
		KeyFile:   "/ssh/id_rsa",
		AddedAt:   expiryClock.Add(-9 * time.Hour),
		ExpiresAt: expiryClock.Add(-time.Hour),
	}}}
	r := runtest.NewRunner().On("ssh-add", runtest.Stdout("", 0))
	e := expirerWith(records, r, &fakeLogger{})
	e.Endpoint = `\\.\pipe\openssh-ssh-agent`

	require.NoError(t, e.ExpireKeys(t.Context()), "ExpireKeys")

	require.Len(t, r.Calls, 1, "one command")
	assert.Equal(t, []string{`SSH_AUTH_SOCK=\\.\pipe\openssh-ssh-agent`}, r.Calls[0].Env,
		"a removal must reach the agent this session was pointed at, not whatever the environment happens to name")
}

func TestRecordsThatCannotBeReadAreReportedRatherThanPassedOver(t *testing.T) {
	records := &fakeAddedKeys{err: errors.New("permission denied")}
	r := runtest.NewRunner()

	err := expirerWith(records, r, &fakeLogger{}).ExpireKeys(t.Context())

	require.Error(t, err, "a store that cannot be read is not an empty store")
	assert.Empty(t, r.Calls, "and nothing is removed on the strength of a list that could not be read")
}
