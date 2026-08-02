package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigReportsWhatIsInForce verifies F35 the way a user checks it: a
// configuration written across several files, then one command asked what SSHakku
// is actually going to do with it.
//
// The values are not the point — Resolve is tested on its own. What is asserted
// here is what only this command can say: which file won, that an exported
// variable beat every file, and that a value SSHakku refused is repeated to the
// person who wrote it instead of being left in the session log.
func TestConfigReportsWhatIsInForce(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.toml", "key_lifetime = \"1h\"\nmax_attempts = 5\n")
	writeConfig(t, home, "config.d/50-work.toml", "key_lifetime = \"2h\"\n")
	writeConfig(t, home, "config.d/90-broken.toml", "giveup_ttl = \"one hour\"\n")

	out, errOut, code := runConfig(t)
	if code != 0 {
		t.Fatalf("exit = %d (%s), want 0: a report changes nothing and cannot fail", code, errOut)
	}

	t.Run("the drop-in that overruled config.toml is named", func(t *testing.T) {
		line := settingLine(t, out, "key_lifetime")
		if !strings.Contains(line, "2h") {
			t.Errorf("%q does not show the value in force", line)
		}
		if !strings.Contains(line, "50-work.toml") {
			t.Errorf("%q does not name the file that set it", line)
		}
	})

	t.Run("a value only config.toml set is attributed to config.toml", func(t *testing.T) {
		line := settingLine(t, out, "max_attempts")
		if !strings.Contains(line, "5") || !strings.Contains(line, "config.toml") {
			t.Errorf("%q does not show 5, set by config.toml", line)
		}
	})

	t.Run("a setting nobody wrote shows its built-in default", func(t *testing.T) {
		line := settingLine(t, out, "command_timeout")
		if !strings.Contains(line, "10s") || !strings.Contains(strings.ToLower(line), "default") {
			t.Errorf("%q does not show the built-in default", line)
		}
	})

	t.Run("a refused value is shown to the person who wrote it", func(t *testing.T) {
		line := settingLine(t, out, "giveup_ttl")
		if !strings.Contains(line, "one hour") {
			t.Errorf("%q does not repeat the value that was refused", line)
		}
		if !strings.Contains(line, "90-broken.toml") {
			t.Errorf("%q does not name the file holding it", line)
		}
	})

	t.Run("the files that were read are listed in the order they were read", func(t *testing.T) {
		first := strings.Index(out, "config.toml")
		work := strings.Index(out, "50-work.toml")
		broken := strings.Index(out, "90-broken.toml")
		if first < 0 || work < 0 || broken < 0 {
			t.Fatalf("not every file is named in the report:\n%s", out)
		}
		if !(first < work && work < broken) {
			t.Errorf("files are not named in reading order:\n%s", out)
		}
	})
}

// TestConfigLetsTheEnvironmentSpeakForItself covers the half of F35 no file can
// show: a variable exported into this shell wins over every file, and the report
// says so rather than naming a file whose value is not in use.
func TestConfigLetsTheEnvironmentSpeakForItself(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.toml", "key_lifetime = \"1h\"\n")
	t.Setenv("SSHAKKU_KEY_LIFETIME", "30m")

	out, _, code := runConfig(t)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	line := settingLine(t, out, "key_lifetime")
	if !strings.Contains(line, "30m") {
		t.Errorf("%q does not show the exported value", line)
	}
	if !strings.Contains(line, "SSHAKKU_KEY_LIFETIME") {
		t.Errorf("%q does not name the variable that set it", line)
	}
}

// TestConfigWithNothingConfiguredStillAnswers covers the account that has never
// written a config file (F35). Every setting still has a value in force, and a
// report that printed nothing would leave the most common case unanswerable.
func TestConfigWithNothingConfiguredStillAnswers(t *testing.T) {
	tempRuntimeEnv(t)

	out, _, code := runConfig(t)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if line := settingLine(t, out, "key_lifetime"); !strings.Contains(line, "8h") {
		t.Errorf("%q does not show the built-in default", line)
	}
	if !strings.Contains(strings.ToLower(out), "no configuration") {
		t.Errorf("the report does not say no file was read:\n%s", out)
	}
}

// runConfig runs `sshakku config` against the environment the test set up,
// returning stdout, stderr and the exit code. No dependency is substituted: the
// command reads the files that are really there, which is the whole subject.
func runConfig(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := deps{}.run(&stdout, &stderr, []string{"config"})
	return stdout.String(), stderr.String(), code
}

// settingLine returns the report's line for one config key.
func settingLine(t *testing.T, report, key string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), key) {
			return line
		}
	}
	t.Fatalf("%q is missing from the report:\n%s", key, report)
	return ""
}

// writeConfig writes one config file under the home's config dir.
func writeConfig(t *testing.T, home, name, body string) {
	t.Helper()
	path := filepath.Join(home, ".config", "sshakku", filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
