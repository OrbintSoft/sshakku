package diagnose

import (
	"bytes"
	"strings"
	"testing"
)

// lineWith returns the line of out that names v, so an assertion can check the
// value sits on that line without pinning the column padding Format uses.
func lineWith(out, v string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, v) {
			return line, true
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
		"SSHAKKU_KEY_LIFETIME": "unset",
	}
	for name, want := range cases {
		line, ok := lineWith(out, name)
		if !ok {
			t.Errorf("report never names %s:\n%s", name, out)
			continue
		}
		if !strings.Contains(line, want) {
			t.Errorf("%s is shown as %q, want it to carry %q", name, line, want)
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
		"SSHAKKU_HANDOFF_TOKEN": "set",
		"SSHAKKU_BW_PASSWORD":   "unset",
	}
	for name, want := range cases {
		line, ok := lineWith(out, name)
		if !ok {
			t.Errorf("report never names %s:\n%s", name, out)
			continue
		}
		if !strings.Contains(line, want) {
			t.Errorf("%s is shown as %q, want it reported %q", name, line, want)
		}
	}
}
