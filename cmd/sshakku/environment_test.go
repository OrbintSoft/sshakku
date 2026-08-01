package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
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
		if got[name] != value {
			t.Errorf("%s reported as %q, want %q", name, got[name], value)
		}
	}
	if _, ok := got["SSH_AUTH_SOCK"]; !ok {
		t.Error("SSH_AUTH_SOCK is not among the variables the report covers")
	}
}

// TestASecretsValueNeverReachesTheReport verifies the half of F31 that matters
// most: a report is what a user pastes into a bug report, so the value of a
// variable that holds a secret must not be anywhere in it — only whether it is
// set. The token is given a value this test can recognise and the whole
// rendered report is searched for it.
func TestASecretsValueNeverReachesTheReport(t *testing.T) {
	const token = "sshakku-a-token-no-report-may-ever-show"
	t.Setenv(keys.EnvPassHandoffToken, token)
	t.Setenv("SSHAKKU_BW_PASSWORD", token)

	shown, secrets := environmentReport()

	for _, v := range shown {
		if strings.Contains(v.Value, token) {
			t.Fatalf("%s carries the secret's value into the report", v.Name)
		}
	}

	set := map[string]bool{}
	for _, s := range secrets {
		set[s.Name] = s.Set
	}
	for _, name := range []string{keys.EnvPassHandoffToken, "SSHAKKU_BW_PASSWORD"} {
		if !set[name] {
			t.Errorf("%s is set in this shell but the report does not say so", name)
		}
	}

	var buf bytes.Buffer
	diagnose.Format(&buf, diagnose.Report{
		FixedSock: "/run/user/1000/sshakku/agent.sock",
		Findings:  []string{"no problems detected"},
		Env:       shown,
		SecretEnv: secrets,
	})
	if strings.Contains(buf.String(), token) {
		t.Errorf("the rendered report contains the secret's value:\n%s", buf.String())
	}
}
