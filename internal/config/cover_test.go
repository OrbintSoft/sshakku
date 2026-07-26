package config

import (
	"reflect"
	"testing"
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

// TestLoadDirGlobError covers LoadDir's early return when the directory glob
// itself is malformed: an unclosed character class makes filepath.Glob report a
// syntax error, which LoadDir surfaces instead of scanning files.
func TestLoadDirGlobError(t *testing.T) {
	f, errs := LoadDir("[")
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want a single glob error", errs)
	}
	if !reflect.DeepEqual(f, File{}) {
		t.Errorf("File = %+v, want the zero File on a glob error", f)
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
