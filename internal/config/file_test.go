package config

import (
	"github.com/OrbintSoft/sshakku/internal/keys"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadValid(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "valid.toml"))
	if err != nil {
		t.Fatalf("Load(valid) error = %v", err)
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "8h" {
		t.Errorf("KeyLifetime = %v, want 8h", f.KeyLifetime)
	}
	if f.MaxAttempts == nil || *f.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %v, want 5", f.MaxAttempts)
	}
	if f.GiveupTTL == nil || *f.GiveupTTL != "30m" {
		t.Errorf("GiveupTTL = %v, want 30m", f.GiveupTTL)
	}
	if f.NoGiveup == nil || !*f.NoGiveup {
		t.Errorf("NoGiveup = %v, want true", f.NoGiveup)
	}
	if f.Quiet == nil || !*f.Quiet {
		t.Errorf("Quiet = %v, want true", f.Quiet)
	}
}

func TestLoadPartialLeavesAbsentKeysNil(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "partial.toml"))
	if err != nil {
		t.Fatalf("Load(partial) error = %v", err)
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "2h" {
		t.Errorf("KeyLifetime = %v, want 2h", f.KeyLifetime)
	}
	if f.MaxAttempts != nil || f.GiveupTTL != nil || f.NoGiveup != nil {
		t.Errorf("absent keys must stay nil, got %+v", f)
	}
}

func TestLoadMissingIsZeroNoError(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("a missing file must not error, got %v", err)
	}
	if !reflect.DeepEqual(f, File{}) {
		t.Errorf("a missing file must give the zero File, got %+v", f)
	}
}

func TestLoadUnknownKeyErrorsButDecodesKnown(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "unknown.toml"))
	if err == nil || !strings.Contains(err.Error(), "bogus_key") {
		t.Fatalf("want an error naming bogus_key, got %v", err)
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "1h" {
		t.Errorf("the recognised key must still decode, got %v", f.KeyLifetime)
	}
}

func TestLoadMalformedErrors(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "malformed.toml"))
	if err == nil {
		t.Fatal("a syntax error must be reported")
	}
	if !reflect.DeepEqual(f, File{}) {
		t.Errorf("a malformed file must give the zero File, got %+v", f)
	}
}

func TestMergeOtherWinsWhenSet(t *testing.T) {
	base := File{KeyLifetime: ptr("1h"), WalletStoreInclude: []string{"id_rsa"}}
	other := File{KeyLifetime: ptr("2h")}
	got := base.Merge(other)
	if got.KeyLifetime == nil || *got.KeyLifetime != "2h" {
		t.Errorf("KeyLifetime = %v, want 2h (other's value)", got.KeyLifetime)
	}
	if !reflect.DeepEqual(got.WalletStoreInclude, []string{"id_rsa"}) {
		t.Errorf("WalletStoreInclude = %v, want base's untouched [id_rsa]", got.WalletStoreInclude)
	}
}

func TestMergeExplicitEmptyListOverrides(t *testing.T) {
	base := File{WalletStoreInclude: []string{"id_rsa"}}
	other := File{WalletStoreInclude: []string{}}
	got := base.Merge(other)
	if got.WalletStoreInclude == nil || len(got.WalletStoreInclude) != 0 {
		t.Errorf("WalletStoreInclude = %v, want an explicit empty list from other", got.WalletStoreInclude)
	}
}

func TestLoadDirMergesInFilenameOrder(t *testing.T) {
	f, errs := LoadDir(filepath.Join("testdata", "confd"))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "2h" {
		t.Errorf("KeyLifetime = %v, want 2h (10-override.toml wins over 00-base.toml)", f.KeyLifetime)
	}
	if !reflect.DeepEqual(f.WalletStoreInclude, []string{"id_rsa"}) {
		t.Errorf("WalletStoreInclude = %v, want [id_rsa] from 00-base.toml (10-override.toml never sets it)", f.WalletStoreInclude)
	}
}

func TestLoadDirExplicitEmptyListOverrides(t *testing.T) {
	f, errs := LoadDir(filepath.Join("testdata", "confd-emptylist"))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if f.WalletStoreInclude == nil || len(f.WalletStoreInclude) != 0 {
		t.Errorf("WalletStoreInclude = %v, want an explicit empty list from 10-clear.toml", f.WalletStoreInclude)
	}
}

func TestLoadDirSkipsMalformedFileButKeepsOthers(t *testing.T) {
	f, errs := LoadDir(filepath.Join("testdata", "confd-malformed"))
	if len(errs) != 1 {
		t.Fatalf("want exactly one error (the malformed file), got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "10-bad.toml") {
		t.Errorf("error %v must name the offending file", errs[0])
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "3h" {
		t.Errorf("KeyLifetime = %v, want 3h from 00-good.toml despite 10-bad.toml", f.KeyLifetime)
	}
	if f.Quiet == nil || !*f.Quiet {
		t.Errorf("Quiet = %v, want true from 20-good2.toml despite 10-bad.toml", f.Quiet)
	}
}

func TestLoadDirUnknownKeyErrorsButKeepsRecognisedField(t *testing.T) {
	f, errs := LoadDir(filepath.Join("testdata", "confd-unknown"))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "bogus_key") {
		t.Fatalf("want one error naming bogus_key, got %v", errs)
	}
	if f.KeyLifetime == nil || *f.KeyLifetime != "1h" {
		t.Errorf("KeyLifetime = %v, want 1h (the recognised field must still merge)", f.KeyLifetime)
	}
}

func TestLoadDirMissingIsZeroNoError(t *testing.T) {
	f, errs := LoadDir(filepath.Join("testdata", "does-not-exist-dir"))
	if len(errs) != 0 {
		t.Fatalf("a missing dir must not error, got %v", errs)
	}
	if !reflect.DeepEqual(f, File{}) {
		t.Errorf("a missing dir must give the zero File, got %+v", f)
	}
}

func TestResolveDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := Settings{
		KeyLifetime: DefaultKeyLifetime, GiveupTTL: DefaultGiveupTTL,
		CommandTimeout: keys.DefaultCommandTimeout, InteractiveTimeout: keys.DefaultInteractiveTimeout,
		WalletStoreMode: WalletStoreModeAll, AutoLoadMode: AutoLoadModeAll, SecretBackend: platformDefaultSecretBackend,
		// Resolved even with nothing configured: every path that builds an
		// entry name reads it from here, so an empty one would leave each of
		// them to supply a default of its own.
		ServicePrefix: keys.DefaultServicePrefix,
		// No route named means SSHakku chooses one per platform; only this
		// value ever falls back.
		KeePassXCRoute: KeePassXCRouteAuto,
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("Resolve(empty) = %+v, want %+v", s, want)
	}
}

func TestResolveFileWins(t *testing.T) {
	file := File{
		KeyLifetime: ptr("2h"),
		MaxAttempts: ptr(5),
		GiveupTTL:   ptr("30m"),
		NoGiveup:    ptr(true),
		Quiet:       ptr(true),
	}
	s, errs := Resolve(file, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := Settings{
		KeyLifetime: 2 * time.Hour,
		MaxAttempts: 5,
		GiveupTTL:   30 * time.Minute,
		NoGiveup:    true,
		Quiet:       true,
		// Not set in the file above: every command is bounded whether or not
		// the user configured a budget.
		CommandTimeout:     keys.DefaultCommandTimeout,
		InteractiveTimeout: keys.DefaultInteractiveTimeout,
		WalletStoreMode:    WalletStoreModeAll,
		AutoLoadMode:       AutoLoadModeAll,
		SecretBackend:      platformDefaultSecretBackend,
		ServicePrefix:      keys.DefaultServicePrefix,
		KeePassXCRoute:     KeePassXCRouteAuto,
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("Resolve(file) = %+v, want %+v", s, want)
	}
}

func TestResolveWalletStoreModeDefaultsToAll(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.WalletStoreMode != WalletStoreModeAll {
		t.Errorf("WalletStoreMode = %q, want %q", s.WalletStoreMode, WalletStoreModeAll)
	}
}

func TestResolveWalletStoreModeFromFile(t *testing.T) {
	for _, mode := range []string{WalletStoreModeAll, WalletStoreModeInclude, WalletStoreModeExclude} {
		file := File{WalletStoreMode: ptr(mode)}
		s, errs := Resolve(file, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("mode %q: unexpected errors: %v", mode, errs)
		}
		if s.WalletStoreMode != mode {
			t.Errorf("mode %q: WalletStoreMode = %q", mode, s.WalletStoreMode)
		}
	}
}

func TestResolveWalletStoreModeInvalidFallsBackToAll(t *testing.T) {
	file := File{WalletStoreMode: ptr("bogus")}
	s, errs := Resolve(file, lookupFrom(nil))
	if len(errs) == 0 {
		t.Fatal("an invalid wallet_store_mode must be reported")
	}
	if s.WalletStoreMode != WalletStoreModeAll {
		t.Errorf("WalletStoreMode = %q, want %q on an invalid value", s.WalletStoreMode, WalletStoreModeAll)
	}
}

func TestResolveWalletStoreListsPassThrough(t *testing.T) {
	file := File{
		WalletStoreMode:    ptr(WalletStoreModeInclude),
		WalletStoreInclude: []string{"id_rsa", "id_ed25519"},
		WalletStoreExclude: []string{"id_ignored"},
	}
	s, _ := Resolve(file, lookupFrom(nil))
	if !reflect.DeepEqual(s.WalletStoreInclude, []string{"id_rsa", "id_ed25519"}) {
		t.Errorf("WalletStoreInclude = %v", s.WalletStoreInclude)
	}
	if !reflect.DeepEqual(s.WalletStoreExclude, []string{"id_ignored"}) {
		t.Errorf("WalletStoreExclude = %v", s.WalletStoreExclude)
	}
}

func TestStoresWalletAllModeStoresEverything(t *testing.T) {
	s := Settings{WalletStoreMode: WalletStoreModeAll}
	if !s.StoresWallet("anything") {
		t.Error("mode all must store every key")
	}
}

func TestStoresWalletIncludeModeConsultsOnlyInclude(t *testing.T) {
	s := Settings{
		WalletStoreMode:    WalletStoreModeInclude,
		WalletStoreInclude: []string{"id_rsa"},
		WalletStoreExclude: []string{"id_rsa"}, // must be ignored: mode is authoritative
	}
	if !s.StoresWallet("id_rsa") {
		t.Error("id_rsa is in the include list, must store")
	}
	if s.StoresWallet("id_ed25519") {
		t.Error("id_ed25519 is not in the include list, must not store")
	}
}

func TestStoresWalletExcludeModeConsultsOnlyExclude(t *testing.T) {
	s := Settings{
		WalletStoreMode:    WalletStoreModeExclude,
		WalletStoreInclude: []string{"id_ed25519"}, // must be ignored: mode is authoritative
		WalletStoreExclude: []string{"id_rsa"},
	}
	if s.StoresWallet("id_rsa") {
		t.Error("id_rsa is in the exclude list, must not store")
	}
	if !s.StoresWallet("id_ed25519") {
		t.Error("id_ed25519 is not in the exclude list, must store")
	}
}

func TestResolveAutoLoadModeDefaultsToAll(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.AutoLoadMode != AutoLoadModeAll {
		t.Errorf("AutoLoadMode = %q, want %q", s.AutoLoadMode, AutoLoadModeAll)
	}
}

func TestResolveAutoLoadModeFromFile(t *testing.T) {
	for _, mode := range []string{AutoLoadModeAll, AutoLoadModeInclude, AutoLoadModeExclude} {
		file := File{AutoLoadMode: ptr(mode)}
		s, errs := Resolve(file, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("mode %q: unexpected errors: %v", mode, errs)
		}
		if s.AutoLoadMode != mode {
			t.Errorf("mode %q: AutoLoadMode = %q", mode, s.AutoLoadMode)
		}
	}
}

func TestResolveAutoLoadModeInvalidFallsBackToAll(t *testing.T) {
	file := File{AutoLoadMode: ptr("bogus")}
	s, errs := Resolve(file, lookupFrom(nil))
	if len(errs) == 0 {
		t.Fatal("an invalid auto_load_mode must be reported")
	}
	if s.AutoLoadMode != AutoLoadModeAll {
		t.Errorf("AutoLoadMode = %q, want %q on an invalid value", s.AutoLoadMode, AutoLoadModeAll)
	}
}

func TestResolveAutoLoadListsPassThrough(t *testing.T) {
	file := File{
		AutoLoadMode:    ptr(AutoLoadModeInclude),
		AutoLoadInclude: []string{"id_rsa", "id_ed25519"},
		AutoLoadExclude: []string{"id_ignored"},
	}
	s, _ := Resolve(file, lookupFrom(nil))
	if !reflect.DeepEqual(s.AutoLoadInclude, []string{"id_rsa", "id_ed25519"}) {
		t.Errorf("AutoLoadInclude = %v", s.AutoLoadInclude)
	}
	if !reflect.DeepEqual(s.AutoLoadExclude, []string{"id_ignored"}) {
		t.Errorf("AutoLoadExclude = %v", s.AutoLoadExclude)
	}
}

func TestAutoLoadsAllModeLoadsEverything(t *testing.T) {
	s := Settings{AutoLoadMode: AutoLoadModeAll}
	if !s.AutoLoads("anything") {
		t.Error("mode all must load every key")
	}
}

func TestAutoLoadsIncludeModeConsultsOnlyInclude(t *testing.T) {
	s := Settings{
		AutoLoadMode:    AutoLoadModeInclude,
		AutoLoadInclude: []string{"id_rsa"},
		AutoLoadExclude: []string{"id_rsa"}, // must be ignored: mode is authoritative
	}
	if !s.AutoLoads("id_rsa") {
		t.Error("id_rsa is in the include list, must load")
	}
	if s.AutoLoads("id_ed25519") {
		t.Error("id_ed25519 is not in the include list, must not load")
	}
}

func TestAutoLoadsExcludeModeConsultsOnlyExclude(t *testing.T) {
	s := Settings{
		AutoLoadMode:    AutoLoadModeExclude,
		AutoLoadInclude: []string{"id_ed25519"}, // must be ignored: mode is authoritative
		AutoLoadExclude: []string{"id_rsa"},
	}
	if s.AutoLoads("id_rsa") {
		t.Error("id_rsa is in the exclude list, must not load")
	}
	if !s.AutoLoads("id_ed25519") {
		t.Error("id_ed25519 is not in the exclude list, must load")
	}
}

func TestResolveEnvOverridesFile(t *testing.T) {
	file := File{KeyLifetime: ptr("2h"), MaxAttempts: ptr(2)}
	env := map[string]string{
		"SSHAKKU_KEY_LIFETIME": "15m",
		"SSHAKKU_MAX_ATTEMPTS": "7",
	}
	s, errs := Resolve(file, lookupFrom(env))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.KeyLifetime != 15*time.Minute {
		t.Errorf("KeyLifetime = %v, want 15m (env wins)", s.KeyLifetime)
	}
	if s.MaxAttempts != 7 {
		t.Errorf("MaxAttempts = %d, want 7 (env wins)", s.MaxAttempts)
	}
}

func TestResolveEnvCanOverrideBoolToFalse(t *testing.T) {
	file := File{Quiet: ptr(true)}
	s, _ := Resolve(file, lookupFrom(map[string]string{"SSHAKKU_QUIET": "0"}))
	if s.Quiet {
		t.Error("SSHAKKU_QUIET=0 must override quiet = true in the file")
	}
}

func TestResolveInvalidEnvMaxAttemptsFallsToFile(t *testing.T) {
	file := File{MaxAttempts: ptr(4)}
	s, _ := Resolve(file, lookupFrom(map[string]string{"SSHAKKU_MAX_ATTEMPTS": "0"}))
	if s.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4 (invalid env falls through to file)", s.MaxAttempts)
	}
}

// TestSecretBackendsIsTheOneList guards what the exported answers are for:
// every caller that offers the user a choice of wallet, or turns one down,
// reads them here instead of keeping a copy of its own. Two copies is how the
// diagnostics came to accept a name the configuration would not.
func TestSecretBackendsIsTheOneList(t *testing.T) {
	names := SecretBackends()
	if len(names) == 0 {
		t.Fatal("this system offers no wallet at all")
	}
	for _, name := range names {
		if !SecretBackendAvailable(name) {
			t.Errorf("SecretBackendAvailable(%q) = false for a wallet the same package offers", name)
		}
		s, errs := Resolve(File{SecretBackend: ptr(name)}, lookupFrom(nil))
		if len(errs) != 0 || s.SecretBackend != name {
			t.Errorf("naming %q resolved to %q with errors %v; an offered wallet must be accepted", name, s.SecretBackend, errs)
		}
	}
	if SecretBackendAvailable("bogus") {
		t.Error(`SecretBackendAvailable("bogus") = true, want false`)
	}
	if !SecretBackendAvailable(DefaultSecretBackend()) {
		t.Errorf("the default wallet %q is not among the ones offered", DefaultSecretBackend())
	}

	// The returned slice is the caller's own: handing out the live one would
	// let any caller reorder or empty the list every other caller reads.
	names[0] = "tampered"
	if SecretBackends()[0] == "tampered" {
		t.Error("SecretBackends hands out the list itself, so a caller can change what every other caller sees")
	}
}

func TestResolveSecretBackendDefaultsToThePlatformWallet(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.SecretBackend != platformDefaultSecretBackend {
		t.Errorf("SecretBackend = %q, want %q", s.SecretBackend, platformDefaultSecretBackend)
	}
}

// TestResolveSecretBackendFromFile walks whatever this system offers rather
// than a written-out list, so it cannot drift from the list the product
// actually consults. Which names those are on each platform is pinned in
// backend_platform_linux_test.go and backend_platform_other_test.go.
func TestResolveSecretBackendFromFile(t *testing.T) {
	for _, backend := range platformSecretBackends {
		file := File{SecretBackend: ptr(backend)}
		s, errs := Resolve(file, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("backend %q: unexpected errors: %v", backend, errs)
		}
		if s.SecretBackend != backend {
			t.Errorf("backend %q: SecretBackend = %q", backend, s.SecretBackend)
		}
	}
}

func TestResolveSecretBackendInvalidFallsBackToThePlatformWallet(t *testing.T) {
	file := File{SecretBackend: ptr("bogus")}
	s, errs := Resolve(file, lookupFrom(nil))
	if len(errs) == 0 {
		t.Fatal("an invalid secret_backend must be reported")
	}
	if s.SecretBackend != platformDefaultSecretBackend {
		t.Errorf("SecretBackend = %q, want %q on an invalid value", s.SecretBackend, platformDefaultSecretBackend)
	}
}

// TestResolveSecretBackendFromEitherPlatformsWallets checks the rule itself
// against both platforms' tables, from whichever platform is running it.
//
// The tables are per-platform and the code for one is not compiled on the
// other; the rule that reads them is neither, and this is what keeps a change
// to it from being provable on one operating system alone.
func TestResolveSecretBackendFromEitherPlatformsWallets(t *testing.T) {
	const (
		secretService = "secret-service"
		keychain      = "keychain"
	)
	linux := []string{secretService, SecretBackendOnePassword, SecretBackendBitwarden, SecretBackendKeePassXC}
	elsewhere := []string{keychain, SecretBackendOnePassword, SecretBackendBitwarden, SecretBackendKeePassXC}

	tests := []struct {
		name      string
		named     *string
		available []string
		fallback  string
		want      string
		wantErr   string // a fragment the error must contain, "" for no error
	}{
		{name: "nothing named on linux", available: linux, fallback: secretService, want: secretService},
		{name: "nothing named elsewhere", available: elsewhere, fallback: keychain, want: keychain},
		{name: "a wallet this system has", named: ptr(SecretBackendBitwarden), available: elsewhere, fallback: keychain, want: SecretBackendBitwarden},
		{
			name: "the keychain named on linux", named: ptr(keychain), available: linux, fallback: secretService,
			want: secretService, wantErr: keychain,
		},
		{
			name: "the secret service named elsewhere", named: ptr(secretService), available: elsewhere, fallback: keychain,
			want: keychain, wantErr: secretService,
		},
		{
			name: "a name no system has", named: ptr("bogus"), available: linux, fallback: secretService,
			want: secretService, wantErr: "bogus",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSecretBackendFrom(tc.named, tc.available, tc.fallback)
			if got != tc.want {
				t.Errorf("backend = %q, want %q", got, tc.want)
			}
			switch {
			case tc.wantErr == "":
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			case err == nil:
				t.Fatalf("want an error naming %q, got none", tc.wantErr)
			case !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error = %v, want it to name %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveSecretBackendAccountFieldsPassThrough(t *testing.T) {
	file := File{
		SecretBackend:    ptr(SecretBackendBitwarden),
		OnePasswordVault: ptr("sshakku-vault"),
		BitwardenEmail:   ptr("user@example.invalid"),
		BitwardenServer:  ptr("https://vault.example.invalid"),
	}
	s, errs := Resolve(file, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.OnePasswordVault != "sshakku-vault" {
		t.Errorf("OnePasswordVault = %q", s.OnePasswordVault)
	}
	if s.BitwardenEmail != "user@example.invalid" {
		t.Errorf("BitwardenEmail = %q", s.BitwardenEmail)
	}
	if s.BitwardenServer != "https://vault.example.invalid" {
		t.Errorf("BitwardenServer = %q", s.BitwardenServer)
	}
}

func TestResolveSecretBackendAccountFieldsDefaultEmpty(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.OnePasswordVault != "" || s.BitwardenEmail != "" || s.BitwardenServer != "" {
		t.Errorf("absent account fields must default empty, got %+v", s)
	}
}

func TestResolveMalformedEnvDurationReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_KEY_LIFETIME": "banana"}))
	if len(errs) == 0 {
		t.Fatal("a malformed duration must be reported")
	}
	if s.KeyLifetime != DefaultKeyLifetime {
		t.Errorf("KeyLifetime = %v, want the default on a malformed value", s.KeyLifetime)
	}
}
