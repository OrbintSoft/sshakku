package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadSourcesReadsWhatALoginShellReads covers the list of files behind the
// report (F35): what SSHakku read, in the order it read it. The order is the
// answer — with config.d merged by filename, the last file to mention a key is
// the one whose value is in force, and a list that came back in another order
// would name the wrong file.
func TestLoadSourcesReadsWhatALoginShellReads(t *testing.T) {
	t.Run("config.toml first, then config.d in filename order", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":            "key_lifetime = \"1h\"\n",
			"config.d/50-work.toml":  "key_lifetime = \"2h\"\n",
			"config.d/10-first.toml": "key_lifetime = \"3h\"\n",
		})
		want := []string{
			filepath.Join(dir, "config.toml"),
			filepath.Join(dir, "config.d", "10-first.toml"),
			filepath.Join(dir, "config.d", "50-work.toml"),
		}
		assert.Equal(t, want, sourcePaths(LoadSources(dir)), "sources, in the order they were read")
	})

	// A file that is not there was not read, and naming it would say SSHakku
	// consulted something it never opened.
	t.Run("a configuration nobody wrote reads nothing", func(t *testing.T) {
		assert.Empty(t, LoadSources(configDir(t, nil)), "sources")
	})

	t.Run("a file that cannot be parsed is listed with its error", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		sources := LoadSources(dir)
		require.Len(t, sources, 2, "both files must be listed")
		assert.Error(t, sources[0].Err, "the malformed file must come back with an error")
		assert.NoError(t, sources[1].Err, "the readable file must carry no error")
	})
}

// TestExplainNamesWhereEachValueCameFrom is the report itself (F35): every
// setting, the value in force, and what put it there. Nothing else can answer
// it — the merge happens in memory and keeps the value, not its origin.
func TestExplainNamesWhereEachValueCameFrom(t *testing.T) {
	t.Run("a setting nobody wrote is the built-in default", func(t *testing.T) {
		s := explained(t, nil, nil, "key_lifetime")
		assert.Equal(t, OriginDefault, s.From.Kind, "origin must be the built-in default")
		assert.Equal(t, DefaultKeyLifetime.String(), s.Value, "value")
	})

	t.Run("a value in config.toml names config.toml", func(t *testing.T) {
		dir := configDir(t, map[string]string{"config.toml": "key_lifetime = \"1h\"\n"})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		assert.Equal(t, OriginFile, s.From.Kind, "origin kind")
		assert.Equal(t, filepath.Join(dir, "config.toml"), s.From.Name, "origin name")
		assert.Equal(t, "1h0m0s", s.Value, "value must be the file's own")
	})

	// The whole reason this exists: which of several files won is not something
	// a person can work out by reading them.
	t.Run("a drop-in overruling config.toml names the drop-in", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \"1h\"\n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		assert.Equal(t, filepath.Join(dir, "config.d", "50-work.toml"), s.From.Name,
			"origin must be the drop-in that overruled the file")
		assert.Equal(t, "2h0m0s", s.Value, "value must be the drop-in's own")
	})

	t.Run("an exported variable overrules every file and is named", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \"1h\"\n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		env := map[string]string{"SSHAKKU_KEY_LIFETIME": "30m"}
		s := explained(t, LoadSources(dir), env, "key_lifetime")
		assert.Equal(t, OriginEnv, s.From.Kind, "origin kind")
		assert.Equal(t, "SSHAKKU_KEY_LIFETIME", s.From.Name, "origin name")
		assert.Equal(t, "30m0s", s.Value, "value must be the exported one")
	})

	// A setting with no environment variable must never be attributed to one,
	// however the environment is dressed up: a user told the wrong place to
	// change a value looks for a variable that changes nothing.
	t.Run("a config-file-only setting is never attributed to the environment", func(t *testing.T) {
		dir := configDir(t, map[string]string{"config.toml": "wallet_store_mode = \"exclude\"\n"})
		env := map[string]string{"SSHAKKU_WALLET_STORE_MODE": "all"}
		s := explained(t, LoadSources(dir), env, "wallet_store_mode")
		assert.Equal(t, OriginFile, s.From.Kind, "origin must be the file, which is the only place this can be set")
		assert.Equal(t, WalletStoreModeExclude, s.Value, "value must be the file's own")
	})

	// Saying a refused value is in force would be the very lie this report
	// exists to remove — and the file holding it is what the user has to open.
	t.Run("a refused value falls back to the default and says who wrote it", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.d/50-work.toml": "key_lifetime = \"eight hours\"\n",
		})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		assert.Equal(t, OriginDefault, s.From.Kind, "the default must be in force")
		assert.Equal(t, DefaultKeyLifetime.String(), s.Value, "value")
		require.NotNil(t, s.Refused, "something must say the value was refused")
		assert.Equal(t, filepath.Join(dir, "config.d", "50-work.toml"), s.Refused.From.Name,
			"the refusal must name the file that stated it")
		assert.ErrorContains(t, s.Refused.Err, "eight hours", "the refusal must quote the value that was refused")
	})
}

// TestEverySettingIsExplained keeps the report honest as settings are added
// (F35): a promise to print every setting is broken by the next one that
// arrives without a line here, and the arrival is exactly when nobody is
// looking at this file.
func TestEverySettingIsExplained(t *testing.T) {
	reported := map[string]bool{}
	for _, s := range Explain(nil, lookupFrom(nil)) {
		assert.Falsef(t, reported[s.Key], "%s is reported twice", s.Key)
		reported[s.Key] = true
	}

	fields := reflect.TypeOf(File{})
	for i := range fields.NumField() {
		key := fields.Field(i).Tag.Get("toml")
		if key == "" {
			assert.Failf(t, "a setting nobody can name",
				"%s has no toml tag, so no user can set it and no report can name it", fields.Field(i).Name)
			continue
		}
		assert.Truef(t, reported[key], "%s can be configured but is not in the report", key)
		delete(reported, key)
	}
	for key := range reported {
		assert.Failf(t, "a report line nobody can write",
			"the report names %q, which is not a setting anybody can write", key)
	}
}

// explained resolves sources with env and returns the one setting named key.
func explained(t *testing.T, sources []Source, env map[string]string, key string) Setting {
	t.Helper()
	for _, s := range Explain(sources, lookupFrom(env)) {
		if s.Key == key {
			return s
		}
	}
	require.Failf(t, "missing from the report", "%q", key)
	return Setting{}
}

// configDir writes files (paths relative to it, "/" separated) into a fresh
// config directory and returns it.
func configDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return dir
}

func sourcePaths(sources []Source) []string {
	paths := make([]string, len(sources))
	for i, s := range sources {
		paths[i] = s.Path
	}
	return paths
}
