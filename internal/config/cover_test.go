package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

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
