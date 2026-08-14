package config

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/run"
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
	require.NoError(t, err, "Load(valid)")
	assert.Equal(t, ptr("8h"), f.KeyLifetime, "KeyLifetime")
	assert.Equal(t, ptr(5), f.MaxAttempts, "MaxAttempts")
	assert.Equal(t, ptr("30m"), f.GiveupTTL, "GiveupTTL")
	assert.Equal(t, ptr(true), f.NoGiveup, "NoGiveup")
	assert.Equal(t, ptr(true), f.Quiet, "Quiet")
}

func TestLoadPartialLeavesAbsentKeysNil(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "partial.toml"))
	require.NoError(t, err, "Load(partial)")
	assert.Equal(t, ptr("2h"), f.KeyLifetime, "KeyLifetime")
	assert.Nil(t, f.MaxAttempts, "an absent key must stay nil")
	assert.Nil(t, f.GiveupTTL, "an absent key must stay nil")
	assert.Nil(t, f.NoGiveup, "an absent key must stay nil")
}

func TestLoadMissingIsZeroNoError(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "does-not-exist.toml"))
	require.NoError(t, err, "a missing file must not error")
	assert.Equal(t, File{}, f, "a missing file must give the zero File")
}

func TestLoadUnknownKeyErrorsButDecodesKnown(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "unknown.toml"))
	assert.ErrorContains(t, err, "bogus_key", "the error must name the unknown key")
	assert.Equal(t, ptr("1h"), f.KeyLifetime, "the recognised key must still decode")
}

func TestLoadMalformedErrors(t *testing.T) {
	f, err := Load(filepath.Join("testdata", "malformed.toml"))
	assert.Error(t, err, "a syntax error must be reported")
	assert.Equal(t, File{}, f, "a malformed file must give the zero File")
}

func TestMergeOtherWinsWhenSet(t *testing.T) {
	base := File{KeyLifetime: ptr("1h"), WalletStoreInclude: []string{"id_rsa"}}
	other := File{KeyLifetime: ptr("2h")}
	got := base.Merge(other)
	assert.Equal(t, ptr("2h"), got.KeyLifetime, "KeyLifetime must be other's value")
	assert.Equal(t, []string{"id_rsa"}, got.WalletStoreInclude, "base's list must be untouched")
}

func TestMergeExplicitEmptyListOverrides(t *testing.T) {
	base := File{WalletStoreInclude: []string{"id_rsa"}}
	other := File{WalletStoreInclude: []string{}}
	got := base.Merge(other)
	assert.Equal(t, []string{}, got.WalletStoreInclude, "an explicit empty list from other must win")
}

// dropIns reads a fixture configuration directory the way every command does,
// returning what the files merge into and what could not be read — the two
// things a caller of LoadSources goes on to do with it.
func dropIns(t *testing.T, fixture string) (File, []error) {
	t.Helper()
	var errs []error
	sources := LoadSources(filepath.Join("testdata", fixture))
	for _, s := range sources {
		if s.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Path, s.Err))
		}
	}
	return Merged(sources), errs
}

func TestDropInsMergeInFilenameOrder(t *testing.T) {
	f, errs := dropIns(t, "confd")
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, ptr("2h"), f.KeyLifetime, "10-override.toml must win over 00-base.toml")
	assert.Equal(t, []string{"id_rsa"}, f.WalletStoreInclude,
		"the list must come from 00-base.toml, which 10-override.toml never sets")
}

func TestDropInExplicitEmptyListOverrides(t *testing.T) {
	f, errs := dropIns(t, "confd-emptylist")
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, []string{}, f.WalletStoreInclude, "an explicit empty list from 10-clear.toml must win")
}

func TestMalformedDropInIsSkippedAndTheOthersKept(t *testing.T) {
	f, errs := dropIns(t, "confd-malformed")
	require.Len(t, errs, 1, "only the malformed file must be reported")
	assert.ErrorContains(t, errs[0], "10-bad.toml", "the error must name the offending file")
	assert.Equal(t, ptr("3h"), f.KeyLifetime, "00-good.toml must still be read despite 10-bad.toml")
	assert.Equal(t, ptr(true), f.Quiet, "20-good2.toml must still be read despite 10-bad.toml")
}

func TestUnknownKeyInADropInKeepsTheRecognisedFields(t *testing.T) {
	f, errs := dropIns(t, "confd-unknown")
	require.Len(t, errs, 1, "only the unknown key must be reported")
	assert.ErrorContains(t, errs[0], "bogus_key", "the error must name the unknown key")
	assert.Equal(t, ptr("1h"), f.KeyLifetime, "the recognised field must still merge")
}

func TestAConfigDirectoryThatIsNotThereIsNoError(t *testing.T) {
	f, errs := dropIns(t, "does-not-exist-dir")
	require.Empty(t, errs, "a missing dir must not error")
	assert.Equal(t, File{}, f, "a missing dir must give the zero File")
}

func TestResolveDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	want := Settings{
		KeyLifetime: DefaultKeyLifetime, GiveupTTL: DefaultGiveupTTL,
		CommandTimeout: run.DefaultCommandTimeout, InteractiveTimeout: run.DefaultInteractiveTimeout,
		WalletStoreMode: WalletStoreModeAll, AutoLoadMode: AutoLoadModeAll, SecretBackend: platformDefaultSecretBackend,
		// Resolved even with nothing configured: every path that builds an
		// entry name reads it from here, so an empty one would leave each of
		// them to supply a default of its own.
		ServicePrefix: keys.DefaultServicePrefix,
		// No route named means SSHakku chooses one per platform; only this
		// value ever falls back.
		KeePassXCRoute: KeePassXCRouteAuto,
		GUIPrompter:    GUIPrompterAuto,
		OnDismiss:      keys.OnDismissStop,
	}
	assert.Equal(t, want, s, "Resolve(empty)")
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
	require.Empty(t, errs, "unexpected errors")
	want := Settings{
		KeyLifetime: 2 * time.Hour,
		MaxAttempts: 5,
		GiveupTTL:   30 * time.Minute,
		NoGiveup:    true,
		Quiet:       true,
		// Not set in the file above: every command is bounded whether or not
		// the user configured a budget.
		CommandTimeout:     run.DefaultCommandTimeout,
		InteractiveTimeout: run.DefaultInteractiveTimeout,
		WalletStoreMode:    WalletStoreModeAll,
		AutoLoadMode:       AutoLoadModeAll,
		SecretBackend:      platformDefaultSecretBackend,
		ServicePrefix:      keys.DefaultServicePrefix,
		KeePassXCRoute:     KeePassXCRouteAuto,
		GUIPrompter:        GUIPrompterAuto,
		OnDismiss:          keys.OnDismissStop,
	}
	assert.Equal(t, want, s, "Resolve(file)")
}

func TestResolveWalletStoreModeDefaultsToAll(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, WalletStoreModeAll, s.WalletStoreMode, "WalletStoreMode")
}

func TestResolveWalletStoreModeFromFile(t *testing.T) {
	for _, mode := range []string{WalletStoreModeAll, WalletStoreModeInclude, WalletStoreModeExclude} {
		s, errs := Resolve(File{WalletStoreMode: ptr(mode)}, lookupFrom(nil))
		require.Emptyf(t, errs, "mode %q: unexpected errors", mode)
		assert.Equalf(t, mode, s.WalletStoreMode, "mode %q", mode)
	}
}

func TestResolveWalletStoreModeInvalidFallsBackToAll(t *testing.T) {
	s, errs := Resolve(File{WalletStoreMode: ptr("bogus")}, lookupFrom(nil))
	assert.NotEmpty(t, errs, "an invalid wallet_store_mode must be reported")
	assert.Equal(t, WalletStoreModeAll, s.WalletStoreMode, "WalletStoreMode on an invalid value")
}

func TestResolveWalletStoreListsPassThrough(t *testing.T) {
	file := File{
		WalletStoreMode:    ptr(WalletStoreModeInclude),
		WalletStoreInclude: []string{"id_rsa", "id_ed25519"},
		WalletStoreExclude: []string{"id_ignored"},
	}
	s, _ := Resolve(file, lookupFrom(nil))
	assert.Equal(t, []string{"id_rsa", "id_ed25519"}, s.WalletStoreInclude, "WalletStoreInclude")
	assert.Equal(t, []string{"id_ignored"}, s.WalletStoreExclude, "WalletStoreExclude")
}

func TestStoresWalletAllModeStoresEverything(t *testing.T) {
	s := Settings{WalletStoreMode: WalletStoreModeAll}
	assert.True(t, s.StoresWallet("anything"), "mode all must store every key")
}

func TestStoresWalletIncludeModeConsultsOnlyInclude(t *testing.T) {
	s := Settings{
		WalletStoreMode:    WalletStoreModeInclude,
		WalletStoreInclude: []string{"id_rsa"},
		WalletStoreExclude: []string{"id_rsa"}, // must be ignored: mode is authoritative
	}
	assert.True(t, s.StoresWallet("id_rsa"), "id_rsa is in the include list, must store")
	assert.False(t, s.StoresWallet("id_ed25519"), "id_ed25519 is not in the include list, must not store")
}

func TestStoresWalletExcludeModeConsultsOnlyExclude(t *testing.T) {
	s := Settings{
		WalletStoreMode:    WalletStoreModeExclude,
		WalletStoreInclude: []string{"id_ed25519"}, // must be ignored: mode is authoritative
		WalletStoreExclude: []string{"id_rsa"},
	}
	assert.False(t, s.StoresWallet("id_rsa"), "id_rsa is in the exclude list, must not store")
	assert.True(t, s.StoresWallet("id_ed25519"), "id_ed25519 is not in the exclude list, must store")
}

func TestResolveAutoLoadModeDefaultsToAll(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, AutoLoadModeAll, s.AutoLoadMode, "AutoLoadMode")
}

func TestResolveAutoLoadModeFromFile(t *testing.T) {
	for _, mode := range []string{AutoLoadModeAll, AutoLoadModeInclude, AutoLoadModeExclude} {
		s, errs := Resolve(File{AutoLoadMode: ptr(mode)}, lookupFrom(nil))
		require.Emptyf(t, errs, "mode %q: unexpected errors", mode)
		assert.Equalf(t, mode, s.AutoLoadMode, "mode %q", mode)
	}
}

func TestResolveAutoLoadModeInvalidFallsBackToAll(t *testing.T) {
	s, errs := Resolve(File{AutoLoadMode: ptr("bogus")}, lookupFrom(nil))
	assert.NotEmpty(t, errs, "an invalid auto_load_mode must be reported")
	assert.Equal(t, AutoLoadModeAll, s.AutoLoadMode, "AutoLoadMode on an invalid value")
}

func TestResolveAutoLoadListsPassThrough(t *testing.T) {
	file := File{
		AutoLoadMode:    ptr(AutoLoadModeInclude),
		AutoLoadInclude: []string{"id_rsa", "id_ed25519"},
		AutoLoadExclude: []string{"id_ignored"},
	}
	s, _ := Resolve(file, lookupFrom(nil))
	assert.Equal(t, []string{"id_rsa", "id_ed25519"}, s.AutoLoadInclude, "AutoLoadInclude")
	assert.Equal(t, []string{"id_ignored"}, s.AutoLoadExclude, "AutoLoadExclude")
}

func TestAutoLoadsAllModeLoadsEverything(t *testing.T) {
	s := Settings{AutoLoadMode: AutoLoadModeAll}
	assert.True(t, s.AutoLoads("anything"), "mode all must load every key")
}

func TestAutoLoadsIncludeModeConsultsOnlyInclude(t *testing.T) {
	s := Settings{
		AutoLoadMode:    AutoLoadModeInclude,
		AutoLoadInclude: []string{"id_rsa"},
		AutoLoadExclude: []string{"id_rsa"}, // must be ignored: mode is authoritative
	}
	assert.True(t, s.AutoLoads("id_rsa"), "id_rsa is in the include list, must load")
	assert.False(t, s.AutoLoads("id_ed25519"), "id_ed25519 is not in the include list, must not load")
}

func TestAutoLoadsExcludeModeConsultsOnlyExclude(t *testing.T) {
	s := Settings{
		AutoLoadMode:    AutoLoadModeExclude,
		AutoLoadInclude: []string{"id_ed25519"}, // must be ignored: mode is authoritative
		AutoLoadExclude: []string{"id_rsa"},
	}
	assert.False(t, s.AutoLoads("id_rsa"), "id_rsa is in the exclude list, must not load")
	assert.True(t, s.AutoLoads("id_ed25519"), "id_ed25519 is not in the exclude list, must load")
}

func TestResolveEnvOverridesFile(t *testing.T) {
	file := File{KeyLifetime: ptr("2h"), MaxAttempts: ptr(2)}
	env := map[string]string{
		"SSHAKKU_KEY_LIFETIME": "15m",
		"SSHAKKU_MAX_ATTEMPTS": "7",
	}
	s, errs := Resolve(file, lookupFrom(env))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, 15*time.Minute, s.KeyLifetime, "KeyLifetime: the environment wins")
	assert.Equal(t, 7, s.MaxAttempts, "MaxAttempts: the environment wins")
}

func TestResolveEnvCanOverrideBoolToFalse(t *testing.T) {
	file := File{Quiet: ptr(true)}
	s, _ := Resolve(file, lookupFrom(map[string]string{"SSHAKKU_QUIET": "0"}))
	assert.False(t, s.Quiet, "SSHAKKU_QUIET=0 must override quiet = true in the file")
}

func TestResolveInvalidEnvMaxAttemptsFallsToFile(t *testing.T) {
	file := File{MaxAttempts: ptr(4)}
	s, _ := Resolve(file, lookupFrom(map[string]string{"SSHAKKU_MAX_ATTEMPTS": "0"}))
	assert.Equal(t, 4, s.MaxAttempts, "an invalid environment value must fall through to the file")
}

// TestSecretBackendsIsTheOneList and the other tests that need this system to
// have a wallet at all are in wallet_unix_test.go.

func TestResolveSecretBackendDefaultsToThePlatformWallet(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, platformDefaultSecretBackend, s.SecretBackend, "SecretBackend")
}

// TestResolveSecretBackendFromFile walks whatever this system offers rather
// than a written-out list, so it cannot drift from the list the product
// actually consults. Which names those are on each platform is pinned in
// backend_platform_linux_test.go and backend_platform_darwin_test.go.
func TestResolveSecretBackendFromFile(t *testing.T) {
	for _, backend := range platformSecretBackends {
		s, errs := Resolve(File{SecretBackend: ptr(backend)}, lookupFrom(nil))
		require.Emptyf(t, errs, "backend %q: unexpected errors", backend)
		assert.Equalf(t, backend, s.SecretBackend, "backend %q", backend)
	}
}

func TestResolveSecretBackendInvalidFallsBackToThePlatformWallet(t *testing.T) {
	s, errs := Resolve(File{SecretBackend: ptr("bogus")}, lookupFrom(nil))
	assert.NotEmpty(t, errs, "an invalid secret_backend must be reported")
	assert.Equal(t, platformDefaultSecretBackend, s.SecretBackend, "SecretBackend on an invalid value")
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
			assert.Equal(t, tc.want, got, "backend")
			if tc.wantErr == "" {
				assert.NoError(t, err, "unexpected error")
			} else {
				assert.ErrorContains(t, err, tc.wantErr, "the error must name the value that cannot be used")
			}
		})
	}
}

func TestResolveSecretBackendAccountFieldsDefaultEmpty(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Empty(t, s.OnePasswordVault, "OnePasswordVault must default empty")
	assert.Empty(t, s.BitwardenEmail, "BitwardenEmail must default empty")
	assert.Empty(t, s.BitwardenServer, "BitwardenServer must default empty")
}

func TestResolveMalformedEnvDurationReportsAndDefaults(t *testing.T) {
	s, errs := Resolve(File{}, lookupFrom(map[string]string{"SSHAKKU_KEY_LIFETIME": "banana"}))
	assert.NotEmpty(t, errs, "a malformed duration must be reported")
	assert.Equal(t, DefaultKeyLifetime, s.KeyLifetime, "KeyLifetime must be the default on a malformed value")
}

// TestResolveGUIPrompterFrom covers the rule both platforms share, against each
// platform's table: the tables differ, what is done with them does not, so both
// answers stay checkable from either machine.
func TestResolveGUIPrompterFrom(t *testing.T) {
	linux := []string{GUIPrompterAuto, GUIPrompterNone, "pinentry", "kdialog"}
	darwin := []string{GUIPrompterAuto, GUIPrompterNone, "osascript"}

	cases := []struct {
		name      string
		val       *string
		available []string
		want      string
		wantErr   bool
	}{
		{"unset means auto", nil, linux, GUIPrompterAuto, false},
		{"empty means auto", ptr(""), linux, GUIPrompterAuto, false},
		{"a dialog this system has", ptr("kdialog"), linux, "kdialog", false},
		{"refusing a dialog", ptr(GUIPrompterNone), linux, GUIPrompterNone, false},
		{"the other system's dialog", ptr("osascript"), linux, GUIPrompterAuto, true},
		{"and the other way round", ptr("kdialog"), darwin, GUIPrompterAuto, true},
		{"a typo", ptr("pinetry"), linux, GUIPrompterAuto, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveGUIPrompterFrom(c.val, c.available)
			assert.Equal(t, c.want, got, "resolveGUIPrompterFrom")
			if c.wantErr {
				assert.Error(t, err, "a dialog this system has not got must be reported")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}
}
