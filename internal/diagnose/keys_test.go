package diagnose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keystate"
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
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	if len(r.Keys) != 1 {
		t.Fatalf("Keys = %v, want 1 entry", r.Keys)
	}
	kv := r.Keys[0]
	if kv.Name != "id_ed25519" || !kv.Loaded || !kv.Tracked || kv.NoExpiry {
		t.Fatalf("unexpected KeyView: %+v", kv)
	}
	wantExpiry := fixedNow.Add(7 * time.Hour)
	if !kv.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", kv.ExpiresAt, wantExpiry)
	}

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	if !strings.Contains(out, "id_ed25519") || !strings.Contains(out, "expires in 7h0m0s") {
		t.Fatalf("Format output missing expected key/TTL line, got:\n%s", out)
	}
}

func TestGatherKeysNotLoaded(t *testing.T) {
	ks := &KeySource{
		Lister: fakeKeyLister{paths: []string{"/home/u/.ssh/id_rsa"}},
		Fingerprint: fakeKeyFingerprinter{
			byPath:  map[string]string{"/home/u/.ssh/id_rsa": "SHA256:BBB"},
			agentFP: map[string]bool{},
		},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	if len(r.Keys) != 1 || r.Keys[0].Loaded {
		t.Fatalf("Keys = %+v, want one not-loaded key", r.Keys)
	}

	var b strings.Builder
	Format(&b, r)
	if !strings.Contains(b.String(), "id_rsa") || !strings.Contains(b.String(), "not loaded") {
		t.Fatalf("Format output missing not-loaded line, got:\n%s", b.String())
	}
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
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	if len(r.Keys) != 1 || !r.Keys[0].Loaded || !r.Keys[0].NoExpiry {
		t.Fatalf("Keys = %+v, want one loaded, no-expiry key", r.Keys)
	}
	var b strings.Builder
	Format(&b, r)
	if !strings.Contains(b.String(), "no expiry") {
		t.Fatalf("Format output missing 'no expiry', got:\n%s", b.String())
	}
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
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	if len(r.Keys) != 1 || !r.Keys[0].Loaded || r.Keys[0].Tracked {
		t.Fatalf("Keys = %+v, want one loaded, untracked key", r.Keys)
	}
	var b strings.Builder
	Format(&b, r)
	if !strings.Contains(b.String(), "TTL unknown") {
		t.Fatalf("Format output missing 'TTL unknown', got:\n%s", b.String())
	}
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
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	var b strings.Builder
	Format(&b, r)
	out := b.String()
	// The key is still Loaded (the fake agent reports its fingerprint), so the
	// record's elapsed lifetime doesn't mean the agent actually dropped it —
	// report it as no-longer-trustworthy TTL tracking, not a confident
	// "expired" that also wrongly promises a new shell will refill it (it
	// won't: the loader dedups on an already-loaded fingerprint and skips).
	if !strings.Contains(out, "TTL unknown") || !strings.Contains(out, "record expired 1h0m0s ago") {
		t.Fatalf("Format output missing the stale-record TTL-unknown line, got:\n%s", out)
	}
	if strings.Contains(out, "a new shell will refill it") {
		t.Fatalf("Format output makes a false refill promise for a key the loader would just skip, got:\n%s", out)
	}
}

func TestGatherKeysEnumerateError(t *testing.T) {
	ks := &KeySource{Lister: fakeKeyLister{err: errors.New("boom")}}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks)

	if r.KeysErr == nil {
		t.Fatal("KeysErr = nil, want the enumeration error")
	}
	if len(r.Keys) != 0 {
		t.Fatalf("Keys = %v, want none on enumeration error", r.Keys)
	}
}

func TestGatherNilKeySourceSkipsKeysSection(t *testing.T) {
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, nil)
	if r.Keys != nil || r.KeysErr != nil {
		t.Fatalf("Keys/KeysErr = %v/%v, want both zero when KeySource is nil", r.Keys, r.KeysErr)
	}
	var b strings.Builder
	Format(&b, r)
	if strings.Contains(b.String(), "~/.ssh keys") {
		t.Fatalf("Format output must omit the keys section when Keys is empty, got:\n%s", b.String())
	}
}
