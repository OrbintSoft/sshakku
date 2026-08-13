package diagnose

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
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

	assert.Contains(t, out, "environment variables:", "the report must have a section for them")
	cases := map[string]string{
		"SSH_ASKPASS":          "/usr/local/bin/sshakku-askpass",
		"SSH_ASKPASS_REQUIRE":  "force",
		"SSHAKKU_KEY_LIFETIME": "(unset)",
	}
	for name, want := range cases {
		got, ok := envState(out, name)
		// Only compare the state once there is one: a report that never named
		// the variable has already been reported by the assertion above it, and
		// the comparison would add a second failure saying the same thing.
		if assert.Truef(t, ok, "the report must name %s", name) {
			assert.Equalf(t, want, got, "the state %s is shown in", name)
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

	r := Gather(t.Context(), Inputs{
		FixedSock:     fixed,
		OurUID:        1000,
		EnvUnreadable: true,
		Env:           []EnvVar{{Name: "SSH_ASKPASS"}},
		SecretEnv:     []SecretEnvVar{{Name: "SSHAKKU_HANDOFF_TOKEN"}},
	}, src, prober, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "SSH_AUTH_SOCK is unset"),
		"nothing may be concluded about a shell that was never read: %v", r.Findings)
	assert.Falsef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"nothing may be concluded about a shell that was never read: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "cannot be read from here"),
		"the report must say the environment could not be read: %v", r.Findings)

	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()
	for _, name := range []string{"SSH_AUTH_SOCK", "SSH_ASKPASS", "SSHAKKU_HANDOFF_TOKEN"} {
		got, ok := envState(out, name)
		if assert.Truef(t, ok, "the report must name %s", name) {
			assert.Equalf(t, "(not readable from here)", got,
				"%s must be withheld rather than guessed at", name)
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
		if assert.Truef(t, ok, "the report must name %s", name) {
			assert.Equalf(t, want, got, "%s must be reported as set or unset and nothing more", name)
		}
	}
}
