package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileFingerprint(t *testing.T) {
	t.Run("reads the SHA256 field", func(t *testing.T) {
		r := newFakeRunner().on("ssh-keygen", stdout("256 SHA256:abc123 user@host (ED25519)\n", 0))
		fp, err := FileFingerprint(t.Context(), r, "/home/u/.ssh/id_ed25519")
		require.NoError(t, err, "reading a key file's fingerprint must succeed")
		assert.Equal(t, "SHA256:abc123", fp,
			"the fingerprint is what decides whether the agent already holds this key")
		require.NotEmpty(t, r.calls, "ssh-keygen must actually be run")
		assert.Equal(t, []string{"-lf", "/home/u/.ssh/id_ed25519"}, r.calls[0].Args,
			"and asked about exactly the file named")
	})

	t.Run("unreadable key yields empty fingerprint, no error", func(t *testing.T) {
		r := newFakeRunner().on("ssh-keygen", stdout("", 1))
		fp, err := FileFingerprint(t.Context(), r, "/home/u/.ssh/id_rsa")
		require.NoError(t, err, "a key ssh-keygen could not read is not an error: the key may still open")
		assert.Empty(t, fp, "there is simply no fingerprint to compare against what the agent holds")
	})

	t.Run("a failure to start ssh-keygen is an error", func(t *testing.T) {
		wantErr := errors.New("exec: \"ssh-keygen\": not found")
		r := newFakeRunner().on("ssh-keygen", fails(wantErr))
		_, err := FileFingerprint(t.Context(), r, "/home/u/.ssh/id_rsa")
		assert.ErrorIs(t, err, wantErr, "ssh-keygen missing altogether is something the caller has to hear about")
	})
}

// TestFingerprintLookupsAreBounded covers what these two lookups must not do to
// whoever is waiting on them. Both shell out to a program that can stop
// answering — an ssh-add talking to an agent whose socket leads nowhere, an
// ssh-keygen on a filesystem that has gone away — and both are called from the
// login path and from the doctor. A lookup with no deadline turns either of
// those into a shell, or a report, that never comes back.
func TestFingerprintLookupsAreBounded(t *testing.T) {
	t.Run("reading a key file", func(t *testing.T) {
		r := newFakeRunner().on("ssh-keygen", stdout("256 SHA256:abc user@host (ED25519)\n", 0))
		_, err := FileFingerprint(t.Context(), r, "/home/u/.ssh/id_ed25519")
		require.NoError(t, err, "reading a key file's fingerprint must succeed")
		require.NotEmpty(t, r.calls, "ssh-keygen must actually be run")
		assert.Positive(t, r.calls[0].Timeout, "and given a deadline to answer within")
	})

	t.Run("asking the agent", func(t *testing.T) {
		r := newFakeRunner().on("ssh-add", stdout("256 SHA256:abc one (ED25519)\n", 0))
		_, err := AgentFingerprints(t.Context(), r)
		require.NoError(t, err, "asking the agent what it holds must succeed")
		require.NotEmpty(t, r.calls, "ssh-add must actually be run")
		assert.Positive(t, r.calls[0].Timeout, "and given a deadline to answer within")
	})
}

func TestAgentFingerprints(t *testing.T) {
	t.Run("collects every loaded fingerprint", func(t *testing.T) {
		out := "256 SHA256:aaa one (ED25519)\n2048 SHA256:bbb two (RSA)\n"
		r := newFakeRunner().on("ssh-add", stdout(out, 0))
		set, err := AgentFingerprints(t.Context(), r)
		require.NoError(t, err, "asking the agent what it holds must succeed")
		assert.Equal(t, map[string]bool{"SHA256:aaa": true, "SHA256:bbb": true}, set,
			"every key the agent holds must be there, or one of them is loaded a second time")
	})

	t.Run("empty agent yields an empty set, no error", func(t *testing.T) {
		r := newFakeRunner().on("ssh-add", stdout("The agent has no identities.\n", 1))
		set, err := AgentFingerprints(t.Context(), r)
		require.NoError(t, err, "an agent holding nothing is the ordinary state at login, not an error")
		assert.Empty(t, set, "and it holds nothing")
	})

	t.Run("a failure to start ssh-add is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := newFakeRunner().on("ssh-add", fails(wantErr))
		_, err := AgentFingerprints(t.Context(), r)
		assert.ErrorIs(t, err, wantErr,
			"an agent that could not be asked must be reported: every key would otherwise look unloaded")
	})
}

func TestRunnerFingerprinter(t *testing.T) {
	r := newFakeRunner().
		on("ssh-keygen", stdout("256 SHA256:abc123 user@host (ED25519)\n", 0)).
		on("ssh-add", stdout("256 SHA256:abc123 one (ED25519)\n", 0))
	f := RunnerFingerprinter{Runner: r}

	fp, err := f.FileFingerprint(t.Context(), "/home/u/.ssh/id_ed25519")
	require.NoError(t, err, "reading a key file's fingerprint must succeed")
	assert.Equal(t, "SHA256:abc123", fp, "and be the one ssh-keygen reported")
	set, err := f.AgentFingerprints(t.Context())
	require.NoError(t, err, "asking the agent what it holds must succeed")
	assert.Containsf(t, set, "SHA256:abc123", "and the key the agent holds must be in the answer: %v", set)
}

func TestFingerprintField(t *testing.T) {
	cases := map[string]string{
		"256 SHA256:abc user@host (ED25519)": "SHA256:abc",
		"2048 MD5:aa:bb:cc legacy (RSA)":     "MD5:aa:bb:cc",
		"The agent has no identities.":       "",
		"":                                   "",
		"single":                             "",
	}
	for line, want := range cases {
		assert.Equalf(t, want, fingerprintField(line), "fingerprintField(%q)", line)
	}
}
