package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Zerof(t, code, "a report changes nothing and cannot fail: %s", errOut)

	t.Run("the drop-in that overruled config.toml is named", func(t *testing.T) {
		line := settingLine(t, out, "key_lifetime")
		assert.Contains(t, line, "2h", "the value actually in force")
		assert.Contains(t, line, "50-work.toml", "the file that set it, which is the only way to know where to edit")
	})

	t.Run("a value only config.toml set is attributed to config.toml", func(t *testing.T) {
		line := settingLine(t, out, "max_attempts")
		assert.Contains(t, line, "5", "the value in force")
		assert.Contains(t, line, "config.toml", "the file that set it")
	})

	t.Run("a setting nobody wrote shows its built-in default", func(t *testing.T) {
		line := settingLine(t, out, "command_timeout")
		assert.Contains(t, line, "10s", "the built-in value")
		assert.Contains(t, strings.ToLower(line), "default", "and that it is the built-in one, not something a file set")
	})

	t.Run("a refused value is shown to the person who wrote it", func(t *testing.T) {
		line := settingLine(t, out, "giveup_ttl")
		assert.Contains(t, line, "one hour", "the value that was refused, repeated back to whoever wrote it")
		assert.Contains(t, line, "90-broken.toml", "and the file it is still sitting in")
	})

	t.Run("the files that were read are listed in the order they were read", func(t *testing.T) {
		require.Contains(t, out, "config.toml", "the base file")
		require.Contains(t, out, "50-work.toml", "the first drop-in")
		require.Contains(t, out, "90-broken.toml", "the second drop-in")
		// Reading order is what tells the user which file overrides which, so
		// the report has to list them in it.
		assert.Less(t, strings.Index(out, "config.toml"), strings.Index(out, "50-work.toml"),
			"config.toml is read before the drop-ins")
		assert.Less(t, strings.Index(out, "50-work.toml"), strings.Index(out, "90-broken.toml"),
			"drop-ins are read in filename order")
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
	require.Zero(t, code, "a report changes nothing and cannot fail")
	line := settingLine(t, out, "key_lifetime")
	assert.Contains(t, line, "30m", "the exported value wins over every file")
	assert.Contains(t, line, "SSHAKKU_KEY_LIFETIME",
		"and the report must name the variable, not a file whose value is not in use")
}

// TestConfigWithNothingConfiguredStillAnswers covers the account that has never
// written a config file (F35). Every setting still has a value in force, and a
// report that printed nothing would leave the most common case unanswerable.
func TestConfigWithNothingConfiguredStillAnswers(t *testing.T) {
	tempRuntimeEnv(t)

	out, _, code := runConfig(t)
	require.Zero(t, code, "a report changes nothing and cannot fail")
	assert.Contains(t, settingLine(t, out, "key_lifetime"), "8h", "every setting still has a value in force")
	assert.Contains(t, strings.ToLower(out), "no configuration", "and the report must say no file was read")
}

// TestConfigSpellsOutWhatZeroMeans covers the value a report cannot print
// literally without misleading (F35): `key_lifetime = 0` disables expiry, and
// "0s" on its own reads as "expires immediately" just as easily.
func TestConfigSpellsOutWhatZeroMeans(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.toml", "key_lifetime = \"0\"\n")

	out, _, code := runConfig(t)
	require.Zero(t, code, "a report changes nothing and cannot fail")
	assert.Contains(t, settingLine(t, out, "key_lifetime"), "no expiry",
		"a bare 0s reads as expires immediately, which is the opposite of what it does")
}

// TestConfigRefusesArgumentsItDoesNotKnow keeps a mistyped flag from being read
// as a request to print: a user who typed something is owed an answer about
// what they typed, not a report they did not ask for.
func TestConfigRefusesArgumentsItDoesNotKnow(t *testing.T) {
	tempRuntimeEnv(t)

	var stdout, stderr bytes.Buffer
	assert.Equal(t, 2, deps{}.run(t.Context(), &stdout, &stderr, []string{"config", "--sohw"}),
		"an argument the command does not know is a usage error")
	assert.Contains(t, stderr.String(), "--sohw", "the answer must name what was actually typed")
	assert.Empty(t, stdout.String(), "and must not be a report nobody asked for")
}

// TestConfigNamesAFileItCouldNotRead covers the half of F35 about the files
// rather than the settings: one that no longer parses is listed with its own
// error beside its name, so a user with a directory full of drop-ins is not
// left to work out which of them was skipped — while every file that still
// reads goes on applying.
func TestConfigNamesAFileItCouldNotRead(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.toml", "key_lifetime = \"3h\"\n")
	writeConfig(t, home, "config.d/70-stray.toml", "key_lifetime = \n")

	out, _, code := runConfig(t)
	require.Zero(t, code, "a file that cannot be read is reported, not fatal")

	listed := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "70-stray.toml") {
			listed = line
			break
		}
	}
	require.NotEmptyf(t, listed, "the file that could not be read must be named:\n%s", out)
	assert.Contains(t, listed, "(", "naming the file without saying what was wrong with it leaves the user guessing")
	assert.Contains(t, settingLine(t, out, "key_lifetime"), "3h",
		"a file that could not be read must not stop the ones that could from applying")
}

// TestConfigRelativeKeepsAPathItCannotShorten covers the fallback in the naming
// of files: the report names the directory once and each file relative to it,
// but a path that is not inside that directory is printed whole — a relative
// path climbing out of it says less than the path it came from.
func TestConfigRelativeKeepsAPathItCannotShorten(t *testing.T) {
	dir := filepath.Join("home", "someone", ".config", "sshakku")

	inside := filepath.Join(dir, "config.d", "50-work.toml")
	assert.Equal(t, filepath.Join("config.d", "50-work.toml"), configRelative(dir, inside),
		"a file under the directory is named relative to it")

	outside := filepath.Join("etc", "sshakku", "config.toml")
	assert.Equal(t, outside, configRelative(dir, outside),
		"a path that is not inside it is printed whole: a relative path climbing out says less")
}

// TestConfigReportsAFailedWrite covers the report that could not be delivered:
// a command whose whole output failed must not exit 0, or a caller redirecting
// it to a full disk is told everything went well.
func TestConfigReportsAFailedWrite(t *testing.T) {
	tempRuntimeEnv(t)

	var stderr bytes.Buffer
	assert.Equal(t, 1, deps{}.config(t.Context(), errWriter{}, &stderr, nil),
		"a report that was never delivered must not exit as though it had been")
	assert.NotEmpty(t, stderr.String(), "and the failure must be said out loud")
}

// runConfig runs `sshakku config` against the environment the test set up,
// returning stdout, stderr and the exit code. No dependency is substituted: the
// command reads the files that are really there, which is the whole subject.
func runConfig(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := deps{}.run(t.Context(), &stdout, &stderr, []string{"config"})
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
	require.FailNowf(t, "a setting the report must carry is missing", "%q is not in:\n%s", key, report)
	return ""
}

// writeConfig writes one config file under the home's config dir.
func writeConfig(t *testing.T, home, name, body string) {
	t.Helper()
	path := filepath.Join(home, ".config", "sshakku", filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700), "create the config directory")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600), "write the config file")
}
