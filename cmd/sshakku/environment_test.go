package main

import (
	"bytes"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys/handoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvironmentReportReadsTheShellItRunsIn verifies F31: the variables the
// report shows are the ones this shell actually set, not defaults.
func TestEnvironmentReportReadsTheShellItRunsIn(t *testing.T) {
	t.Setenv("SSH_ASKPASS", "/opt/sshakku/bin/sshakku-askpass")
	t.Setenv("SSH_ASKPASS_REQUIRE", "force")
	t.Setenv("SSHAKKU_KEY_LIFETIME", "30m")
	t.Setenv("SSHAKKU_QUIET", "1")

	shown, _ := environmentReport()
	got := map[string]string{}
	for _, v := range shown {
		got[v.Name] = v.Value
	}

	want := map[string]string{
		"SSH_ASKPASS":          "/opt/sshakku/bin/sshakku-askpass",
		"SSH_ASKPASS_REQUIRE":  "force",
		"SSHAKKU_KEY_LIFETIME": "30m",
		"SSHAKKU_QUIET":        "1",
	}
	for name, value := range want {
		assert.Equalf(t, value, got[name], "%s must be reported as this shell set it", name)
	}
	assert.Contains(t, got, "SSH_AUTH_SOCK", "the report must cover the variable everything else depends on")
}

// TestTheNamesForASessionThisProcessCannotReadCarryNoValues covers the report
// about somebody else's session. The names are still worth showing — they say
// what the report would have covered — but every value is withheld. Filled in
// from this process's own environment they would be printed as the target
// user's: wrong about them, and a disclosure of the caller's own session into a
// report that is not about it.
func TestTheNamesForASessionThisProcessCannotReadCarryNoValues(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/run/user/0/keyring/ssh")
	t.Setenv(handoff.EnvToken, "a-token-belonging-to-the-caller")

	shown, secrets := environmentNames()

	require.NotEmpty(t, shown, "the names still say what the report would have covered")
	for _, v := range shown {
		assert.Emptyf(t, v.Value, "%s must carry nothing read from this process's own environment", v.Name)
	}

	require.NotEmpty(t, secrets, "including the ones whose value is a secret")
	for _, s := range secrets {
		assert.Falsef(t, s.Set, "%s must not be reported set on the strength of this process having it", s.Name)
	}
}

// TestASecretsValueNeverReachesTheReport verifies the half of F31 that matters
// most: a report is what a user pastes into a bug report, so the value of a
// variable that holds a secret must not be anywhere in it — only whether it is
// set. The token is given a value this test can recognise and the whole
// rendered report is searched for it.
func TestASecretsValueNeverReachesTheReport(t *testing.T) {
	const token = "sshakku-a-token-no-report-may-ever-show"
	t.Setenv(handoff.EnvToken, token)
	t.Setenv("SSHAKKU_BW_PASSWORD", token)

	shown, secrets := environmentReport()

	for _, v := range shown {
		require.NotContainsf(t, v.Value, token, "%s carries the secret's value into the report", v.Name)
	}

	set := map[string]bool{}
	for _, s := range secrets {
		set[s.Name] = s.Set
	}
	for _, name := range []string{handoff.EnvToken, "SSHAKKU_BW_PASSWORD"} {
		assert.Truef(t, set[name], "%s is set in this shell, and the report must say so", name)
	}

	var buf bytes.Buffer
	diagnose.Format(&buf, diagnose.Report{
		FixedSock: "/run/user/1000/sshakku/agent.sock",
		Findings:  []string{"no problems detected"},
		Env:       shown,
		SecretEnv: secrets,
	})
	assert.NotContains(t, buf.String(), token,
		"a report is what a user pastes into a bug report, so no secret's value may be anywhere in it")
}
