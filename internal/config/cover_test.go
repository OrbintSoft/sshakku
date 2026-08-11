package config

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestOverruledNamesWhatDecidesInstead verifies the part of F36 a person only
// finds out about by being told: editing a key that something later decides is
// an edit with no effect, and the file being edited is the one place that
// cannot say so.
func TestOverruledNamesWhatDecidesInstead(t *testing.T) {
	const mine = "/etc/sshakku/config.toml"
	const dropIn = "/etc/sshakku/config.d/50-work.toml"
	sources := []Source{
		{Path: mine, File: File{KeyLifetime: ptr("1h"), MaxAttempts: ptr(5), Quiet: ptr(true)}},
		{Path: dropIn, File: File{KeyLifetime: ptr("2h")}},
	}
	env := map[string]string{"SSHAKKU_MAX_ATTEMPTS": "9"}
	lookup := func(name string) (string, bool) { v, ok := env[name]; return v, ok }

	got := map[string]Origin{}
	for _, o := range Overruled(sources, mine, lookup) {
		got[o.Key] = o.By
	}

	if assert.Contains(t, got, "key_lifetime", "key_lifetime is overruled by the drop-in applied after it") {
		assert.Equal(t, OriginFile, got["key_lifetime"].Kind, "key_lifetime is overruled by a file")
		assert.Equal(t, dropIn, got["key_lifetime"].Name, "key_lifetime is overruled by the drop-in")
	}
	if assert.Contains(t, got, "max_attempts", "max_attempts is overruled by the exported variable") {
		assert.Equal(t, OriginEnv, got["max_attempts"].Kind, "max_attempts is overruled by the environment")
	}
	assert.NotContains(t, got, "quiet", "this file is what decides quiet, so nothing overrules it")
	assert.NotContains(t, got, "giveup_ttl", "this file does not set giveup_ttl at all")

	// The file being edited need not be among the ones SSHakku read: somebody
	// with no config.toml of their own is given one to write, and nothing in a
	// file that does not exist yet can be overruled.
	assert.Empty(t, Overruled(sources, "/etc/sshakku/nowhere.toml", lookup),
		"a file that was never read has nothing overruled in it")
}

// TestTheReportShowsWhatWasWrittenWhereSomethingWasWritten is the other half of
// F35 from the empty machine below: where a value has been configured, the
// report has to show that value rather than the built-in one it replaced.
func TestTheReportShowsWhatWasWrittenWhereSomethingWasWritten(t *testing.T) {
	file := File{
		KeyLifetime: ptr("3h"),
		MaxAttempts: ptr(7),
		KeyDir:      ptr("/srv/keys"),
		KeyPatterns: []string{"work-*", "id_*"},
	}
	settings, errs := Resolve(file, func(string) (string, bool) { return "", false })
	require.Empty(t, errs, "the written values must be accepted")

	want := map[string]string{
		"key_lifetime": "3h0m0s",
		"max_attempts": "7",
		"key_dir":      "/srv/keys",
		"key_patterns": "work-*, id_*",
	}
	for _, desc := range settingTable {
		if expected, ok := want[desc.key]; ok {
			assert.Equalf(t, expected, desc.value(settings),
				"%s: the report shows the built-in value where the user wrote one", desc.key)
		}
	}

	// A duration of zero is a setting in its own right — no expiry, no
	// give-up window — and "0s" alone reads as "immediately" just as easily
	// as "never", which are opposite instructions to the person reading it.
	zero, _ := Resolve(File{KeyLifetime: ptr("0s")}, func(string) (string, bool) { return "", false })
	for _, desc := range settingTable {
		if desc.key != "key_lifetime" {
			continue
		}
		assert.Contains(t, desc.value(zero), "no expiry",
			"a key_lifetime of zero must spell out what the zero means")
	}
}

// TestEverySettingRendersAValueOnAMachineWithNoConfiguration covers F35 where
// it is easiest to break: the report is read as a statement of what is in
// force, so a line showing nothing where a built-in value is at work reads as
// "off" or "none". The account that has written no configuration at all is the
// one most likely to be reading the report to find out what SSHakku does, and
// several settings answer that with something other than their own zero — a
// lifetime of zero means no expiry, an empty list of patterns means the
// built-in ones.
func TestEverySettingRendersAValueOnAMachineWithNoConfiguration(t *testing.T) {
	settings, errs := Resolve(File{}, func(string) (string, bool) { return "", false })
	require.Empty(t, errs, "resolving an empty configuration must report nothing")

	for _, desc := range settingTable {
		assert.NotEmptyf(t, desc.value(settings),
			"%s renders nothing where nobody has configured anything, want the value in force spelled out", desc.key)
	}
}

// TestKeyDirWrittenAsHome covers the shorthand a person writes in a config file
// for the directory they live in (F34). It is a path SSHakku resolves itself:
// nothing expands a tilde in a file the way a shell does on a command line.
func TestKeyDirWrittenAsHome(t *testing.T) {
	const home = "/home/someone"
	for written, want := range map[string]string{
		"~":              home,
		"~/keys":         home + "/keys",
		"keys":           home + "/keys",
		"/absolute/keys": "/absolute/keys",
	} {
		assert.Equalf(t, want, Settings{KeyDir: written}.KeyEnumerator(home).Dir, "key_dir %q", written)
	}
}

// TestSettingErrorCarriesTheErrorItRefused covers a refusal being recognisable
// by what it wraps rather than by the text it prints, since that is how the
// report tells one refusal from another.
func TestSettingErrorCarriesTheErrorItRefused(t *testing.T) {
	inner := errors.New("not a duration")
	var err error = &SettingError{Key: "key_lifetime", Err: inner}

	assert.ErrorIs(t, err, inner, "the refusal must be recognisable by the error it was made of")
	assert.Equal(t, inner.Error(), err.Error(), "the text must say what was actually wrong")
}

// TestTemplateIsAConfigurationSSHakkuWouldAccept covers what `--edit` puts in
// front of somebody who has no configuration yet (F36). It is offered as a
// starting point, so a template SSHakku would refuse to read is worse than
// none: the first thing the user does with it is save it.
func TestTemplateIsAConfigurationSSHakkuWouldAccept(t *testing.T) {
	dir := configDir(t, map[string]string{"config.toml": Template()})

	sources := LoadSources(dir)
	require.Len(t, sources, 1, "the template must be read as one file")
	require.NoError(t, sources[0].Err, "reading the template back")
	_, errs := Resolve(Merged(sources), func(string) (string, bool) { return "", false })
	assert.Empty(t, errs, "a file that is offered to be saved must be one SSHakku accepts")
}

// TestOnlyTOMLFilesDirectlyInTheDirectoryAreRead covers what a configuration
// directory is allowed to contain besides configuration: editors leave backups
// there, and people keep notes and subdirectories. Reading one of those as
// settings would be a configuration nobody wrote.
func TestOnlyTOMLFilesDirectlyInTheDirectoryAreRead(t *testing.T) {
	dir := configDir(t, map[string]string{
		"config.d/50-work.toml":       "key_lifetime = \"2h\"\n",
		"config.d/50-work.toml.bak":   "key_lifetime = \"99h\"\n",
		"config.d/notes.txt":          "not configuration at all\n",
		"config.d/nested/deeper.toml": "key_lifetime = \"77h\"\n",
	})

	want := []string{filepath.Join(dir, "config.d", "50-work.toml")}
	assert.Equal(t, want, sourcePaths(LoadSources(dir)),
		"only the .toml file directly in the directory must be read")
}

// TestTimeoutsWrittenInAFileAreTheOnesInForce verifies F21's second half: how
// long SSHakku waits is configurable, separately for something expected to
// answer on its own and something waiting on a person. Neither has an
// environment variable, so a file is the only place either can be said — a
// value that does not survive being read from one is a setting that cannot be
// set at all.
//
// It goes the whole way a login shell goes, from files on disk to resolved
// settings. Every other test of these two builds the File by hand and calls
// Resolve, which is how a value that never survived the merge went unnoticed.
func TestTimeoutsWrittenInAFileAreTheOnesInForce(t *testing.T) {
	dir := configDir(t, map[string]string{
		"config.toml":           "command_timeout = \"45s\"\n",
		"config.d/50-work.toml": "interactive_timeout = \"9m\"\n",
	})

	settings, errs := Resolve(Merged(LoadSources(dir)), func(string) (string, bool) { return "", false })
	require.Empty(t, errs, "the written values must be accepted")
	assert.Equal(t, 45*time.Second, settings.CommandTimeout, "command_timeout must be the 45s written in config.toml")
	assert.Equal(t, 9*time.Minute, settings.InteractiveTimeout, "interactive_timeout must be the 9m written in the drop-in")

	// F35: the report exists to end this exact doubt, so a value it shows
	// against a file has to be the value that file put in force. Naming the
	// file beside a number the user never wrote is worse than saying nothing.
	for _, s := range Explain(LoadSources(dir), func(string) (string, bool) { return "", false }) {
		if s.Key != "command_timeout" {
			continue
		}
		assert.Equal(t, "45s", s.Value, "the report must show the value the file put in force")
		assert.Equal(t, OriginFile, s.From.Kind, "the report must name the file that wrote it")
	}
}

// TestOnDismissWrittenInAFileIsTheOneInForce verifies the configurable half of
// F38, the whole way a login shell goes: what closing a passphrase prompt means
// has no environment variable, so a file is the only place it can be said, and a
// value that does not survive being read from one is a setting nobody can set.
// An answer nobody recognises leaves the user asked less rather than more, and
// is reported instead of applied.
func TestOnDismissWrittenInAFileIsTheOneInForce(t *testing.T) {
	noEnv := func(string) (string, bool) { return "", false }

	dir := configDir(t, map[string]string{"config.d/50-work.toml": "on_dismiss = \"retry\"\n"})
	settings, errs := Resolve(Merged(LoadSources(dir)), noEnv)
	require.Empty(t, errs, "the written value must be accepted")
	assert.Equal(t, keys.OnDismissRetry, settings.OnDismiss, "on_dismiss must be what the drop-in wrote")

	// F35: the report is what ends the doubt about which value is in force, so
	// it has to name the file that wrote this one.
	for _, s := range Explain(LoadSources(dir), noEnv) {
		if s.Key != "on_dismiss" {
			continue
		}
		assert.Equal(t, keys.OnDismissRetry, s.Value, "the report must show the value the file put in force")
		assert.Equal(t, OriginFile, s.From.Kind, "the report must name the file that wrote it")
	}

	refused := configDir(t, map[string]string{"config.toml": "on_dismiss = \"whenever\"\n"})
	settings, errs = Resolve(Merged(LoadSources(refused)), noEnv)
	assert.Equal(t, keys.OnDismissStop, settings.OnDismiss, "a value nothing answers to must fall back")
	require.Len(t, errs, 1, "the refused value must be reported once")
	assert.ErrorContains(t, errs[0], "whenever", "the report must name the value that was refused")
}

// TestMergeOtherWinsForEveryField sets every field in both base and other to a
// distinct value, so merging must yield exactly other: it proves other's value
// overrides base's for each key rather than base surviving because other left it
// unset. Comparing the whole struct field by field exercises every field's
// override branch in one go.
//
// The two files are filled by reflection rather than by hand, and a field this
// helper cannot fill fails the test. A file written out field by field silently
// stops covering the ones added after it, which is what happened here: the
// settings for the key directory, the container, the service prefix and the
// dialog were all merged by code no test had ever run.
func TestMergeOtherWinsForEveryField(t *testing.T) {
	base := filledFile(t, "base")
	other := filledFile(t, "other")

	got := base.Merge(other)

	gotV, wantV := reflect.ValueOf(got), reflect.ValueOf(other)
	for i := range gotV.NumField() {
		name := gotV.Type().Field(i).Name
		assert.Equalf(t, wantV.Field(i).Interface(), gotV.Field(i).Interface(),
			"%s did not survive the merge: the value written in the later file is not the one in force, and nothing reports that it was dropped", name)
	}
}

// filledFile returns a File with every field set to a value made from mark, so
// that no two files built this way share one. Whatever kinds of field File
// grows, this has to keep filling them: one it cannot fill would leave that
// field's merge unexercised while the test still passed.
func filledFile(t *testing.T, mark string) File {
	t.Helper()

	var f File
	v := reflect.ValueOf(&f).Elem()
	for i := range v.NumField() {
		field, name := v.Field(i), v.Type().Field(i).Name
		switch field.Interface().(type) {
		case *string:
			field.Set(reflect.ValueOf(ptr(mark + "_" + name)))
		case *int:
			field.Set(reflect.ValueOf(ptr(len(mark) + i)))
		case *bool:
			field.Set(reflect.ValueOf(ptr(mark == "other")))
		case []string:
			field.Set(reflect.ValueOf([]string{mark + "_" + name}))
		default:
			require.Failf(t, "a field this test cannot fill",
				"%s is a %s: a field it cannot fill is one whose merge nothing here covers", name, field.Type())
		}
	}
	return f
}

// TestDropInDirThatCannotBeRead covers the drop-in directory that is there and
// still unreadable — here a plain file where the directory should be, which is
// what somebody who meant to write one file ends up with. It is not the absent
// directory every account has, so it is reported rather than passed over in
// silence, against the directory itself: no single file can be blamed for it.
func TestDropInDirThatCannotBeRead(t *testing.T) {
	dir := configDir(t, map[string]string{"config.d": "key_lifetime = \"1h\"\n"})

	sources := LoadSources(dir)
	require.Len(t, sources, 1, "the unreadable directory must be reported")
	assert.Error(t, sources[0].Err, "the unreadable directory must come back with an error")
	assert.Equal(t, File{}, Merged(sources), "a directory that could not be read must contribute no settings")
}

// TestResolveMalformedGiveupTTLReportsAndDefaults covers Resolve's error branch
// for the give-up TTL: a malformed environment value is reported yet the setting
// falls back to its default.
func TestResolveMalformedGiveupTTLReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_GIVEUP_TTL": "banana"}))
	assert.NotEmpty(t, errs, "a malformed give-up TTL must be reported")
	assert.Equal(t, DefaultGiveupTTL, s.GiveupTTL, "GiveupTTL must be the default on a malformed value")
}

// TestResolveMalformedCommandTimeoutReportsAndDefaults covers Resolve's error
// branch for the command budget. Reporting matters as much as the fallback: a
// user who mistypes the value gets the default silently unless Resolve hands the
// error back, and a wrong budget is invisible until something hangs.
func TestResolveMalformedCommandTimeoutReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_COMMAND_TIMEOUT": "banana"}))
	assert.NotEmpty(t, errs, "a malformed command timeout must be reported")
	assert.Equal(t, keys.DefaultCommandTimeout, s.CommandTimeout, "CommandTimeout must be the default on a malformed value")
}

// TestResolveMalformedInteractiveTimeoutReportsAndDefaults covers Resolve's
// error branch for the interactive budget, the counterpart of the command one
// above.
func TestResolveMalformedInteractiveTimeoutReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_INTERACTIVE_TIMEOUT": "banana"}))
	assert.NotEmpty(t, errs, "a malformed interactive timeout must be reported")
	assert.Equal(t, keys.DefaultInteractiveTimeout, s.InteractiveTimeout, "InteractiveTimeout must be the default on a malformed value")
}
