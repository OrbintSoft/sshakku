package diagnose

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
)

// envState returns what out shows for the variable name: the text after the
// name's colon, with the surrounding padding stripped. Exact rather than a
// substring search, because "set" is a substring of "unset" and a report that
// inverted the two would satisfy a looser assertion.
func envState(out, name string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		_, rest, found := strings.Cut(line, name+":")
		if found {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}

// TestFormatShowsEveryVariableItWasGiven verifies F31: the report shows every
// variable SSHakku reads, each with the value the shell gave it, and "unset"
// where that shell set none.
func TestFormatShowsEveryVariableItWasGiven(t *testing.T) {
	r := Report{
		FixedSock: fixed,
		Findings:  []string{"no problems detected"},
		Env: []EnvVar{
			{Name: "SSH_ASKPASS", Value: "/usr/local/bin/sshakku-askpass"},
			{Name: "SSH_ASKPASS_REQUIRE", Value: "force"},
			{Name: "SSHAKKU_KEY_LIFETIME", Value: ""},
		},
	}
	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "environment variables:") {
		t.Errorf("report has no environment variables section:\n%s", out)
	}
	cases := map[string]string{
		"SSH_ASKPASS":          "/usr/local/bin/sshakku-askpass",
		"SSH_ASKPASS_REQUIRE":  "force",
		"SSHAKKU_KEY_LIFETIME": "(unset)",
	}
	for name, want := range cases {
		got, ok := envState(out, name)
		if !ok {
			t.Errorf("report never names %s:\n%s", name, out)
			continue
		}
		if got != want {
			t.Errorf("%s is shown as %q, want %q", name, got, want)
		}
	}
}

// TestAnUnreadableEnvironmentIsNotReportedAsUnset verifies F31 for a report
// about somebody else's session: their environment cannot be read from here,
// and an unread variable must not be shown as one their shell did not set, nor
// have a conclusion drawn from it. "Unset" is a fact about a shell somebody
// looked at; this report looked at none.
func TestAnUnreadableEnvironmentIsNotReportedAsUnset(t *testing.T) {
	src := fakeSource{procs: []agent.AgentProc{{PID: 100, UID: 1000, Socket: fixed}}}
	prober := fakeProber{up: map[string]bool{fixed: true}}

	r := Gather(Inputs{
		FixedSock:     fixed,
		OurUID:        1000,
		EnvUnreadable: true,
		Env:           []EnvVar{{Name: "SSH_ASKPASS"}},
		SecretEnv:     []SecretEnvVar{{Name: "SSHAKKU_HANDOFF_TOKEN"}},
	}, src, prober, nil, nil, nil, nil)

	if hasFinding(r, "SSH_AUTH_SOCK is unset") {
		t.Errorf("the report claims SSH_AUTH_SOCK is unset for a shell it never read: %v", r.Findings)
	}
	if hasFinding(r, "SSH_ASKPASS is not wired") {
		t.Errorf("the report claims the askpass is not wired for a shell it never read: %v", r.Findings)
	}
	if !hasFinding(r, "cannot be read from here") {
		t.Errorf("findings = %v, want one saying the environment could not be read", r.Findings)
	}

	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()
	for _, name := range []string{"SSH_AUTH_SOCK", "SSH_ASKPASS", "SSHAKKU_HANDOFF_TOKEN"} {
		got, ok := envState(out, name)
		if !ok {
			t.Errorf("report never names %s:\n%s", name, out)
			continue
		}
		if got != "(not readable from here)" {
			t.Errorf("%s is shown as %q, want it withheld rather than guessed at", name, got)
		}
	}
}

// TestFormatSaysWhetherASecretIsSetAndNothingMore verifies the second half of
// F31: a variable whose value is a secret is reported as set or unset, and the
// report has nowhere to print the value itself.
func TestFormatSaysWhetherASecretIsSetAndNothingMore(t *testing.T) {
	r := Report{
		FixedSock: fixed,
		Findings:  []string{"no problems detected"},
		SecretEnv: []SecretEnvVar{
			{Name: "SSHAKKU_HANDOFF_TOKEN", Set: true},
			{Name: "SSHAKKU_BW_PASSWORD", Set: false},
		},
	}
	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()

	cases := map[string]string{
		"SSHAKKU_HANDOFF_TOKEN": "(set)",
		"SSHAKKU_BW_PASSWORD":   "(unset)",
	}
	for name, want := range cases {
		got, ok := envState(out, name)
		if !ok {
			t.Errorf("report never names %s:\n%s", name, out)
			continue
		}
		if got != want {
			t.Errorf("%s is reported %q, want %q", name, got, want)
		}
	}
}
