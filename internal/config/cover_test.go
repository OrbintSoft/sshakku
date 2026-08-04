package config

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

	if by, ok := got["key_lifetime"]; !ok || by.Kind != OriginFile || by.Name != dropIn {
		t.Errorf("key_lifetime overruled by %+v (present: %t), want the drop-in applied after it", by, ok)
	}
	if by, ok := got["max_attempts"]; !ok || by.Kind != OriginEnv {
		t.Errorf("max_attempts overruled by %+v (present: %t), want the exported variable", by, ok)
	}
	if by, ok := got["quiet"]; ok {
		t.Errorf("quiet reported as overruled by %+v, want nothing: this file is what decides it", by)
	}
	if by, ok := got["giveup_ttl"]; ok {
		t.Errorf("giveup_ttl reported as overruled by %+v, want nothing: this file does not set it at all", by)
	}

	// The file being edited need not be among the ones SSHakku read: somebody
	// with no config.toml of their own is given one to write, and nothing in a
	// file that does not exist yet can be overruled.
	if o := Overruled(sources, "/etc/sshakku/nowhere.toml", lookup); len(o) != 0 {
		t.Errorf("Overruled = %+v for a file that was never read, want nothing", o)
	}
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
	if len(errs) != 0 {
		t.Fatalf("Resolve reported %v, want the written values accepted", errs)
	}

	want := map[string]string{
		"key_lifetime": "3h0m0s",
		"max_attempts": "7",
		"key_dir":      "/srv/keys",
		"key_patterns": "work-*, id_*",
	}
	for _, desc := range settingTable {
		if expected, ok := want[desc.key]; ok {
			if got := desc.value(settings); got != expected {
				t.Errorf("%s reads %q, want %q: the report shows the built-in value where the user wrote one", desc.key, got, expected)
			}
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
		if got := desc.value(zero); !strings.Contains(got, "no expiry") {
			t.Errorf("key_lifetime of zero reads %q, want what the zero means spelled out", got)
		}
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
	if len(errs) != 0 {
		t.Fatalf("resolving an empty configuration reported %v, want none", errs)
	}

	for _, desc := range settingTable {
		if got := desc.value(settings); got == "" {
			t.Errorf("%s renders nothing where nobody has configured anything, want the value in force spelled out", desc.key)
		}
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
		if got := (Settings{KeyDir: written}.KeyEnumerator(home).Dir); got != want {
			t.Errorf("key_dir %q reaches %q, want %q", written, got, want)
		}
	}
}

// TestSettingErrorCarriesTheErrorItRefused covers a refusal being recognisable
// by what it wraps rather than by the text it prints, since that is how the
// report tells one refusal from another.
func TestSettingErrorCarriesTheErrorItRefused(t *testing.T) {
	inner := errors.New("not a duration")
	var err error = &SettingError{Key: "key_lifetime", Err: inner}

	if !errors.Is(err, inner) {
		t.Errorf("errors.Is = false for the error the refusal was made of")
	}
	if err.Error() != inner.Error() {
		t.Errorf("Error = %q, want what was actually wrong: %q", err.Error(), inner.Error())
	}
}

// TestTemplateIsAConfigurationSSHakkuWouldAccept covers what `--edit` puts in
// front of somebody who has no configuration yet (F36). It is offered as a
// starting point, so a template SSHakku would refuse to read is worse than
// none: the first thing the user does with it is save it.
func TestTemplateIsAConfigurationSSHakkuWouldAccept(t *testing.T) {
	dir := configDir(t, map[string]string{"config.toml": Template()})

	sources := LoadSources(dir)
	if len(sources) != 1 {
		t.Fatalf("the template was read as %d sources, want one file", len(sources))
	}
	if sources[0].Err != nil {
		t.Fatalf("reading the template back: %v", sources[0].Err)
	}
	if _, errs := Resolve(Merged(sources), func(string) (string, bool) { return "", false }); len(errs) != 0 {
		t.Errorf("the template resolves with %v, want a file that is offered to be saved to be one SSHakku accepts", errs)
	}
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

	got := sourcePaths(LoadSources(dir))
	want := []string{filepath.Join(dir, "config.d", "50-work.toml")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sources = %v, want only the .toml file directly in the directory: %v", got, want)
	}
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
	if len(errs) != 0 {
		t.Fatalf("Resolve reported %v, want the written values accepted", errs)
	}
	if settings.CommandTimeout != 45*time.Second {
		t.Errorf("command_timeout = %v, want the 45s written in config.toml", settings.CommandTimeout)
	}
	if settings.InteractiveTimeout != 9*time.Minute {
		t.Errorf("interactive_timeout = %v, want the 9m written in the drop-in", settings.InteractiveTimeout)
	}

	// F35: the report exists to end this exact doubt, so a value it shows
	// against a file has to be the value that file put in force. Naming the
	// file beside a number the user never wrote is worse than saying nothing.
	for _, s := range Explain(LoadSources(dir), func(string) (string, bool) { return "", false }) {
		if s.Key != "command_timeout" {
			continue
		}
		if s.Value != "45s" || s.From.Kind != OriginFile {
			t.Errorf("the report says command_timeout is %q from kind %d, want 45s from the file that wrote it", s.Value, s.From.Kind)
		}
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
	if len(errs) != 0 {
		t.Fatalf("Resolve reported %v, want the written value accepted", errs)
	}
	if settings.OnDismiss != keys.OnDismissRetry {
		t.Errorf("on_dismiss = %q, want the %q written in the drop-in", settings.OnDismiss, keys.OnDismissRetry)
	}

	// F35: the report is what ends the doubt about which value is in force, so
	// it has to name the file that wrote this one.
	for _, s := range Explain(LoadSources(dir), noEnv) {
		if s.Key == "on_dismiss" && (s.Value != keys.OnDismissRetry || s.From.Kind != OriginFile) {
			t.Errorf("the report says on_dismiss is %q from kind %d, want %q from the file that wrote it",
				s.Value, s.From.Kind, keys.OnDismissRetry)
		}
	}

	refused := configDir(t, map[string]string{"config.toml": "on_dismiss = \"whenever\"\n"})
	settings, errs = Resolve(Merged(LoadSources(refused)), noEnv)
	if settings.OnDismiss != keys.OnDismissStop {
		t.Errorf("on_dismiss = %q for a value nothing answers to, want %q", settings.OnDismiss, keys.OnDismissStop)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "whenever") {
		t.Errorf("errors = %v, want one naming the value that was refused", errs)
	}
}

// TestMergeOtherWinsForEveryField sets every field in both base and other to a
// distinct value, so merging must yield exactly other: it proves other's value
// overrides base's for each key rather than base surviving because other left it
// unset. reflect.DeepEqual across the whole struct exercises every field's
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
		if !reflect.DeepEqual(gotV.Field(i).Interface(), wantV.Field(i).Interface()) {
			t.Errorf("%s did not survive the merge: the value written in the later file is not the one in force, and nothing reports that it was dropped", name)
		}
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
			t.Fatalf("%s is a %s, which this test does not know how to fill: a field it cannot fill is one whose merge nothing here covers", name, field.Type())
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
	if len(sources) != 1 {
		t.Fatalf("sources = %v, want the unreadable directory reported", sourcePaths(sources))
	}
	if sources[0].Err == nil {
		t.Errorf("%s came back without an error", sources[0].Path)
	}
	if !reflect.DeepEqual(Merged(sources), File{}) {
		t.Error("a directory that could not be read must contribute no settings")
	}
}

// TestResolveMalformedGiveupTTLReportsAndDefaults covers Resolve's error branch
// for the give-up TTL: a malformed environment value is reported yet the setting
// falls back to its default.
func TestResolveMalformedGiveupTTLReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_GIVEUP_TTL": "banana"}))
	if len(errs) == 0 {
		t.Fatal("a malformed give-up TTL must be reported")
	}
	if s.GiveupTTL != DefaultGiveupTTL {
		t.Errorf("GiveupTTL = %v, want the default on a malformed value", s.GiveupTTL)
	}
}

// TestResolveMalformedCommandTimeoutReportsAndDefaults covers Resolve's error
// branch for the command budget. Reporting matters as much as the fallback: a
// user who mistypes the value gets the default silently unless Resolve hands the
// error back, and a wrong budget is invisible until something hangs.
func TestResolveMalformedCommandTimeoutReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_COMMAND_TIMEOUT": "banana"}))
	if len(errs) == 0 {
		t.Fatal("a malformed command timeout must be reported")
	}
	if s.CommandTimeout != keys.DefaultCommandTimeout {
		t.Errorf("CommandTimeout = %v, want the default on a malformed value", s.CommandTimeout)
	}
}

// TestResolveMalformedInteractiveTimeoutReportsAndDefaults covers Resolve's
// error branch for the interactive budget, the counterpart of the command one
// above.
func TestResolveMalformedInteractiveTimeoutReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_INTERACTIVE_TIMEOUT": "banana"}))
	if len(errs) == 0 {
		t.Fatal("a malformed interactive timeout must be reported")
	}
	if s.InteractiveTimeout != keys.DefaultInteractiveTimeout {
		t.Errorf("InteractiveTimeout = %v, want the default on a malformed value", s.InteractiveTimeout)
	}
}
