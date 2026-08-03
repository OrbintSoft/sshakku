package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		got := sourcePaths(LoadSources(dir))
		want := []string{
			filepath.Join(dir, "config.toml"),
			filepath.Join(dir, "config.d", "10-first.toml"),
			filepath.Join(dir, "config.d", "50-work.toml"),
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("sources =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})

	// A file that is not there was not read, and naming it would say SSHakku
	// consulted something it never opened.
	t.Run("a configuration nobody wrote reads nothing", func(t *testing.T) {
		if got := LoadSources(configDir(t, nil)); len(got) != 0 {
			t.Errorf("sources = %v, want none", sourcePaths(got))
		}
	})

	t.Run("a file that cannot be parsed is listed with its error", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		sources := LoadSources(dir)
		if len(sources) != 2 {
			t.Fatalf("sources = %v, want both files listed", sourcePaths(sources))
		}
		if sources[0].Err == nil {
			t.Error("the malformed file came back without an error")
		}
		if sources[1].Err != nil {
			t.Errorf("the readable file carries %v, want no error", sources[1].Err)
		}
	})
}

// TestExplainNamesWhereEachValueCameFrom is the report itself (F35): every
// setting, the value in force, and what put it there. Nothing else can answer
// it — the merge happens in memory and keeps the value, not its origin.
func TestExplainNamesWhereEachValueCameFrom(t *testing.T) {
	t.Run("a setting nobody wrote is the built-in default", func(t *testing.T) {
		s := explained(t, nil, nil, "key_lifetime")
		if s.From.Kind != OriginDefault {
			t.Errorf("origin = %v, want the built-in default", s.From)
		}
		if s.Value != DefaultKeyLifetime.String() {
			t.Errorf("value = %q, want %q", s.Value, DefaultKeyLifetime)
		}
	})

	t.Run("a value in config.toml names config.toml", func(t *testing.T) {
		dir := configDir(t, map[string]string{"config.toml": "key_lifetime = \"1h\"\n"})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		if s.From.Kind != OriginFile || s.From.Name != filepath.Join(dir, "config.toml") {
			t.Errorf("origin = %v, want config.toml", s.From)
		}
		if s.Value != "1h0m0s" {
			t.Errorf("value = %q, want the file's own", s.Value)
		}
	})

	// The whole reason this exists: which of several files won is not something
	// a person can work out by reading them.
	t.Run("a drop-in overruling config.toml names the drop-in", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \"1h\"\n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		if s.From.Name != filepath.Join(dir, "config.d", "50-work.toml") {
			t.Errorf("origin = %v, want the drop-in that overruled the file", s.From)
		}
		if s.Value != "2h0m0s" {
			t.Errorf("value = %q, want the drop-in's own", s.Value)
		}
	})

	t.Run("an exported variable overrules every file and is named", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.toml":           "key_lifetime = \"1h\"\n",
			"config.d/50-work.toml": "key_lifetime = \"2h\"\n",
		})
		env := map[string]string{"SSHAKKU_KEY_LIFETIME": "30m"}
		s := explained(t, LoadSources(dir), env, "key_lifetime")
		if s.From.Kind != OriginEnv || s.From.Name != "SSHAKKU_KEY_LIFETIME" {
			t.Errorf("origin = %v, want the environment variable", s.From)
		}
		if s.Value != "30m0s" {
			t.Errorf("value = %q, want the exported one", s.Value)
		}
	})

	// A setting with no environment variable must never be attributed to one,
	// however the environment is dressed up: a user told the wrong place to
	// change a value looks for a variable that changes nothing.
	t.Run("a config-file-only setting is never attributed to the environment", func(t *testing.T) {
		dir := configDir(t, map[string]string{"config.toml": "wallet_store_mode = \"exclude\"\n"})
		env := map[string]string{"SSHAKKU_WALLET_STORE_MODE": "all"}
		s := explained(t, LoadSources(dir), env, "wallet_store_mode")
		if s.From.Kind != OriginFile {
			t.Errorf("origin = %v, want the file, which is the only place this can be set", s.From)
		}
		if s.Value != WalletStoreModeExclude {
			t.Errorf("value = %q, want the file's own", s.Value)
		}
	})

	// Saying a refused value is in force would be the very lie this report
	// exists to remove — and the file holding it is what the user has to open.
	t.Run("a refused value falls back to the default and says who wrote it", func(t *testing.T) {
		dir := configDir(t, map[string]string{
			"config.d/50-work.toml": "key_lifetime = \"eight hours\"\n",
		})
		s := explained(t, LoadSources(dir), nil, "key_lifetime")
		if s.From.Kind != OriginDefault || s.Value != DefaultKeyLifetime.String() {
			t.Errorf("value/origin = %q/%v, want the default in force", s.Value, s.From)
		}
		if s.Refused == nil {
			t.Fatal("nothing says the value was refused")
		}
		if s.Refused.From.Name != filepath.Join(dir, "config.d", "50-work.toml") {
			t.Errorf("refused by %v, want the file that stated it", s.Refused.From)
		}
		if !strings.Contains(s.Refused.Err.Error(), "eight hours") {
			t.Errorf("refusal %v does not quote the value that was refused", s.Refused.Err)
		}
	})
}

// TestEverySettingIsExplained keeps the report honest as settings are added
// (F35): a promise to print every setting is broken by the next one that
// arrives without a line here, and the arrival is exactly when nobody is
// looking at this file.
func TestEverySettingIsExplained(t *testing.T) {
	reported := map[string]bool{}
	for _, s := range Explain(nil, lookupFrom(nil)) {
		if reported[s.Key] {
			t.Errorf("%s is reported twice", s.Key)
		}
		reported[s.Key] = true
	}

	fields := reflect.TypeOf(File{})
	for i := range fields.NumField() {
		key := fields.Field(i).Tag.Get("toml")
		if key == "" {
			t.Errorf("%s has no toml tag, so no user can set it and no report can name it", fields.Field(i).Name)
			continue
		}
		if !reported[key] {
			t.Errorf("%s can be configured but is not in the report", key)
		}
		delete(reported, key)
	}
	for key := range reported {
		t.Errorf("the report names %q, which is not a setting anybody can write", key)
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
	t.Fatalf("%q is missing from the report", key)
	return Setting{}
}

// configDir writes files (paths relative to it, "/" separated) into a fresh
// config directory and returns it.
func configDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
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
