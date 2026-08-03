package config

import (
	"reflect"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestMergeOtherWinsForEveryField sets every field in both base and other to a
// distinct value, so merging must yield exactly other: it proves other's value
// overrides base's for each key rather than base surviving because other left it
// unset. reflect.DeepEqual across the whole struct exercises every field's
// override branch in one go.
func TestMergeOtherWinsForEveryField(t *testing.T) {
	base := File{
		KeyLifetime:        ptr("1h"),
		MaxAttempts:        ptr(1),
		GiveupTTL:          ptr("1h"),
		NoGiveup:           ptr(false),
		Quiet:              ptr(false),
		WalletStoreMode:    ptr("all"),
		WalletStoreInclude: []string{"base_in"},
		WalletStoreExclude: []string{"base_ex"},
		AutoLoadMode:       ptr("all"),
		AutoLoadInclude:    []string{"base_al_in"},
		AutoLoadExclude:    []string{"base_al_ex"},
		SecretBackend:      ptr("secretservice"),
		OnePasswordVault:   ptr("base_vault"),
		BitwardenEmail:     ptr("base@example.com"),
		BitwardenServer:    ptr("base.example.com"),
	}
	other := File{
		KeyLifetime:        ptr("2h"),
		MaxAttempts:        ptr(2),
		GiveupTTL:          ptr("2h"),
		NoGiveup:           ptr(true),
		Quiet:              ptr(true),
		WalletStoreMode:    ptr("include"),
		WalletStoreInclude: []string{"other_in"},
		WalletStoreExclude: []string{"other_ex"},
		AutoLoadMode:       ptr("exclude"),
		AutoLoadInclude:    []string{"other_al_in"},
		AutoLoadExclude:    []string{"other_al_ex"},
		SecretBackend:      ptr("onepassword"),
		OnePasswordVault:   ptr("other_vault"),
		BitwardenEmail:     ptr("other@example.com"),
		BitwardenServer:    ptr("other.example.com"),
	}
	got := base.Merge(other)
	if !reflect.DeepEqual(got, other) {
		t.Errorf("Merge = %+v, want every field overridden by other %+v", got, other)
	}
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
