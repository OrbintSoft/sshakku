package diagnose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keystate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKeyLister returns a fixed list of key paths (or an error).
type fakeKeyLister struct {
	paths []string
	err   error
}

func (f fakeKeyLister) Keys() ([]string, error) { return f.paths, f.err }

// fakeKeyFingerprinter maps key file paths to fingerprints and scripts the
// agent's loaded set.
type fakeKeyFingerprinter struct {
	byPath   map[string]string
	agentFP  map[string]bool
	agentErr error
}

func (f fakeKeyFingerprinter) FileFingerprint(path string) (string, error) {
	return f.byPath[path], nil
}

func (f fakeKeyFingerprinter) AgentFingerprints() (map[string]bool, error) {
	return f.agentFP, f.agentErr
}

// fakeKeyStateSource scripts keystate.Record lookups by key name.
type fakeKeyStateSource map[string]keystate.Record

func (f fakeKeyStateSource) Load(key string) (keystate.Record, bool) {
	rec, ok := f[key]
	return rec, ok
}

func withFixedNow(t *testing.T, fixed time.Time) {
	t.Helper()
	orig := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = orig })
}

func TestGatherKeysLoadedAndTracked(t *testing.T) {
	fixedNow := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	withFixedNow(t, fixedNow)

	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_ed25519"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_ed25519": "SHA256:AAA"},
			agentFP: map[string]bool{"SHA256:AAA": true},
		},
		State: fakeKeyStateSource{
			"id_ed25519": keystate.Record{AddedAt: fixedNow.Add(-1 * time.Hour), Lifetime: 8 * time.Hour},
		},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	require.Len(t, r.Keys, 1, "the one key the directory held")
	kv := r.Keys[0]
	// Four separate things the report says about this key; assert, so one run
	// names every one it got wrong rather than only the first.
	assert.Equal(t, "id_ed25519", kv.Name, "the key's name")
	assert.True(t, kv.Loaded, "the agent reported this key's fingerprint")
	assert.True(t, kv.Tracked, "a key with a record is one whose TTL is known")
	assert.False(t, kv.NoExpiry, "a record carrying a lifetime is one that expires")
	assert.Equal(t, fixedNow.Add(7*time.Hour), kv.ExpiresAt, "what is left of an 8h lifetime an hour in")

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	assert.Contains(t, out, "id_ed25519", "the report must name the key")
	assert.Contains(t, out, "expires in 7h0m0s", "the report must say how long the key has left")
}

func TestGatherKeysNotLoaded(t *testing.T) {
	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_rsa"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_rsa": "SHA256:BBB"},
			agentFP: map[string]bool{},
		},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	require.Len(t, r.Keys, 1, "the one key the directory held")
	assert.False(t, r.Keys[0].Loaded, "the agent did not report this key's fingerprint")

	var b strings.Builder
	Format(&b, r)
	assert.Contains(t, b.String(), "id_rsa", "the report must name the key")
	assert.Contains(t, b.String(), "not loaded", "the report must say the agent does not hold it")
}

func TestGatherKeysLoadedNoExpiry(t *testing.T) {
	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_rsa"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_rsa": "SHA256:CCC"},
			agentFP: map[string]bool{"SHA256:CCC": true},
		},
		State: fakeKeyStateSource{
			"id_rsa": keystate.Record{AddedAt: time.Now(), Lifetime: 0},
		},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	require.Len(t, r.Keys, 1, "the one key the directory held")
	assert.True(t, r.Keys[0].Loaded, "the agent reported this key's fingerprint")
	assert.True(t, r.Keys[0].NoExpiry, "a record with no lifetime is one that never expires")

	var b strings.Builder
	Format(&b, r)
	assert.Contains(t, b.String(), "no expiry", "the report must say the key will not expire")
}

func TestGatherKeysLoadedUntracked(t *testing.T) {
	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_rsa"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_rsa": "SHA256:DDD"},
			agentFP: map[string]bool{"SHA256:DDD": true},
		},
		// no State collaborator: even a loaded key's TTL is unknown.
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	require.Len(t, r.Keys, 1, "the one key the directory held")
	assert.True(t, r.Keys[0].Loaded, "the agent reported this key's fingerprint")
	assert.False(t, r.Keys[0].Tracked, "with nothing recording when it was added, its TTL is not known")

	var b strings.Builder
	Format(&b, r)
	assert.Contains(t, b.String(), "TTL unknown", "the report must say the TTL is not known")
}

func TestGatherKeysExpired(t *testing.T) {
	fixedNow := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	withFixedNow(t, fixedNow)

	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_rsa"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_rsa": "SHA256:EEE"},
			agentFP: map[string]bool{"SHA256:EEE": true},
		},
		State: fakeKeyStateSource{
			"id_rsa": keystate.Record{AddedAt: fixedNow.Add(-9 * time.Hour), Lifetime: 8 * time.Hour},
		},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	// The key is still Loaded (the fake agent reports its fingerprint), so the
	// record's elapsed lifetime doesn't mean the agent actually dropped it —
	// report it as no-longer-trustworthy TTL tracking, not a confident
	// "expired" that also wrongly promises a new shell will refill it (it
	// won't: the loader dedups on an already-loaded fingerprint and skips).
	assert.Contains(t, out, "TTL unknown", "a record that outlived its lifetime no longer says what the TTL is")
	assert.Contains(t, out, "record expired 1h0m0s ago", "the report must say how stale the record is")
	assert.NotContains(t, out, "a new shell will refill it",
		"a key the loader would skip must not be promised a refill")
}

func TestGatherKeysEnumerateError(t *testing.T) {
	ks := &KeySource{Lister: fakeKeyLister{err: errors.New("boom")}}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	assert.Error(t, r.KeysErr, "a directory that could not be listed must be reported")
	assert.Empty(t, r.Keys, "nothing may be listed from a directory that could not be read")
}

func TestFormatNamesTheDirectoryItReadWhenItHeldNoKey(t *testing.T) {
	ks := &KeySource{Dir: "/home/u/work-keys", Lister: fakeKeyLister{}}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)

	var b strings.Builder
	Format(&b, r)
	// A name rule that matches nothing and a directory that is not the one the
	// user meant produce the same empty answer, and the directory's name is the
	// only thing in the report that tells them apart.
	assert.Contains(t, b.String(), "keys in /home/u/work-keys (0)",
		"the report must name the directory it read even when it held no key")
}

func TestGatherNilKeySourceSkipsKeysSection(t *testing.T) {
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, nil, nil)
	assert.Nil(t, r.Keys, "with nowhere to look, no key is listed")
	assert.NoError(t, r.KeysErr, "not looking is not a failure to look")

	var b strings.Builder
	Format(&b, r)
	assert.NotContains(t, b.String(), "keys in", "the report must leave out a section it has nothing for")
}
