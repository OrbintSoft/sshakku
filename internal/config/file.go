package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// File mirrors the on-disk TOML config. Each field is a pointer so an absent key
// stays nil, letting Resolve tell "unset in the file" from "set to the zero
// value" and apply the precedence environment variable > file > default.
//
// The wallet_store_* and auto_load_* keys have no environment-variable
// counterpart: they are config-file only, since the include/exclude lists
// don't fit a single environment variable cleanly.
type File struct {
	KeyLifetime *string `toml:"key_lifetime"`
	MaxAttempts *int    `toml:"max_attempts"`
	GiveupTTL   *string `toml:"giveup_ttl"`
	NoGiveup    *bool   `toml:"no_giveup"`
	Quiet       *bool   `toml:"quiet"`

	WalletStoreMode    *string  `toml:"wallet_store_mode"`
	WalletStoreInclude []string `toml:"wallet_store_include"`
	WalletStoreExclude []string `toml:"wallet_store_exclude"`

	AutoLoadMode    *string  `toml:"auto_load_mode"`
	AutoLoadInclude []string `toml:"auto_load_include"`
	AutoLoadExclude []string `toml:"auto_load_exclude"`

	// SecretBackend and the three fields below are config-file only, for the
	// same reason as wallet_store_mode/auto_load_mode: which backend to use,
	// and its account identity, don't fit a single environment variable
	// cleanly, and an env var would leave the account identity (an email
	// address, a vault name) sitting in the process environment for no
	// benefit over the file.
	SecretBackend    *string `toml:"secret_backend"`
	OnePasswordVault *string `toml:"onepassword_vault"`
	BitwardenEmail   *string `toml:"bitwarden_email"`
	BitwardenServer  *string `toml:"bitwarden_server"`
}

// Settings is the configuration resolved from environment, file, and defaults.
type Settings struct {
	KeyLifetime time.Duration // 0 disables agent key expiry
	MaxAttempts int           // 0 lets the loader use its own default
	GiveupTTL   time.Duration // 0 never expires the give-up record
	NoGiveup    bool          // true disables give-up tracking entirely
	Quiet       bool          // true suppresses the failure notice

	// WalletStoreMode is one of "all", "include", or "exclude"; see StoresWallet.
	WalletStoreMode    string
	WalletStoreInclude []string // key names consulted only when mode is "include"
	WalletStoreExclude []string // key names consulted only when mode is "exclude"

	// AutoLoadMode is one of "all", "include", or "exclude"; see AutoLoads.
	AutoLoadMode    string
	AutoLoadInclude []string // key names consulted only when mode is "include"
	AutoLoadExclude []string // key names consulted only when mode is "exclude"

	// SecretBackend selects which SecretBackend implementation the caller
	// should construct; one of the SecretBackend* constants.
	SecretBackend string
	// OnePasswordVault is the vault name or ID passed to OnePasswordBackend;
	// consulted only when SecretBackend is SecretBackendOnePassword.
	OnePasswordVault string
	// BitwardenEmail and BitwardenServer are passed to BitwardenBackend;
	// consulted only when SecretBackend is SecretBackendBitwarden.
	// BitwardenServer is empty for the default bitwarden.com.
	BitwardenEmail  string
	BitwardenServer string
}

// Wallet-store policy modes for Settings.WalletStoreMode.
const (
	WalletStoreModeAll     = "all"
	WalletStoreModeInclude = "include"
	WalletStoreModeExclude = "exclude"
)

// Auto-load policy modes for Settings.AutoLoadMode.
const (
	AutoLoadModeAll     = "all"
	AutoLoadModeInclude = "include"
	AutoLoadModeExclude = "exclude"
)

// Secret backend choices for Settings.SecretBackend.
const (
	SecretBackendSecretService = "secret-service"
	SecretBackendOnePassword   = "1password"
	SecretBackendBitwarden     = "bitwarden"
	SecretBackendKeychain      = "keychain"
)

// StoresWallet reports whether keyname's passphrase should be persisted to the
// secret store under the resolved wallet-store policy. Mode is authoritative, so
// include and exclude never conflict: "include" consults only WalletStoreInclude
// and "exclude" consults only WalletStoreExclude; the other list, if set, is
// ignored. Any other mode (including the default "all") stores every key.
func (s Settings) StoresWallet(keyname string) bool {
	switch s.WalletStoreMode {
	case WalletStoreModeInclude:
		return containsKey(s.WalletStoreInclude, keyname)
	case WalletStoreModeExclude:
		return !containsKey(s.WalletStoreExclude, keyname)
	default:
		return true
	}
}

// AutoLoads reports whether keyname should be proactively added to the agent
// at shell-init under the resolved auto-load policy. Mode is authoritative, so
// include and exclude never conflict: "include" consults only AutoLoadInclude
// and "exclude" consults only AutoLoadExclude; the other list, if set, is
// ignored. Any other mode (including the default "all") loads every key.
// Independent of StoresWallet: an excluded key can still be loaded on demand
// via the askpass broker, which does not consult this policy.
func (s Settings) AutoLoads(keyname string) bool {
	switch s.AutoLoadMode {
	case AutoLoadModeInclude:
		return containsKey(s.AutoLoadInclude, keyname)
	case AutoLoadModeExclude:
		return !containsKey(s.AutoLoadExclude, keyname)
	default:
		return true
	}
}

func containsKey(keys []string, keyname string) bool {
	for _, k := range keys {
		if k == keyname {
			return true
		}
	}
	return false
}

// Merge returns f with every field other sets applied on top, so other takes
// precedence for any key it sets while f's value survives for a key other
// leaves unset. A pointer field counts as set when non-nil; a slice field
// (the wallet_store_*/auto_load_* lists) counts as set when non-nil, which is
// how the TOML decoder leaves a key that never appeared in the source —
// including other explicitly setting a list to [] overrides f's list with an
// empty one, rather than being indistinguishable from "not mentioned".
func (f File) Merge(other File) File {
	merged := f

	if other.KeyLifetime != nil {
		merged.KeyLifetime = other.KeyLifetime
	}
	if other.MaxAttempts != nil {
		merged.MaxAttempts = other.MaxAttempts
	}
	if other.GiveupTTL != nil {
		merged.GiveupTTL = other.GiveupTTL
	}
	if other.NoGiveup != nil {
		merged.NoGiveup = other.NoGiveup
	}
	if other.Quiet != nil {
		merged.Quiet = other.Quiet
	}

	if other.WalletStoreMode != nil {
		merged.WalletStoreMode = other.WalletStoreMode
	}
	if other.WalletStoreInclude != nil {
		merged.WalletStoreInclude = other.WalletStoreInclude
	}
	if other.WalletStoreExclude != nil {
		merged.WalletStoreExclude = other.WalletStoreExclude
	}

	if other.AutoLoadMode != nil {
		merged.AutoLoadMode = other.AutoLoadMode
	}
	if other.AutoLoadInclude != nil {
		merged.AutoLoadInclude = other.AutoLoadInclude
	}
	if other.AutoLoadExclude != nil {
		merged.AutoLoadExclude = other.AutoLoadExclude
	}

	if other.SecretBackend != nil {
		merged.SecretBackend = other.SecretBackend
	}
	if other.OnePasswordVault != nil {
		merged.OnePasswordVault = other.OnePasswordVault
	}
	if other.BitwardenEmail != nil {
		merged.BitwardenEmail = other.BitwardenEmail
	}
	if other.BitwardenServer != nil {
		merged.BitwardenServer = other.BitwardenServer
	}

	return merged
}

// LoadDir reads every *.toml file directly under dir, in lexicographic
// filename order, merging each on top of the ones before it (Merge) so a
// later file overrides a key an earlier one set. A dir with no matching
// files — including one that doesn't exist — returns the zero File and no
// error, the same "nothing configured" case Load gives for a missing single
// file. A malformed or partially-unrecognised file contributes whatever Load
// decoded from it (zero for a syntax error, the recognised fields for an
// unrecognised-key file) and its error is collected, path-tagged, rather
// than aborting the rest of the directory.
func LoadDir(dir string) (File, []error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return File{}, []error{err}
	}
	sort.Strings(matches)

	var merged File
	var errs []error
	for _, path := range matches {
		f, err := Load(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
		merged = merged.Merge(f)
	}
	return merged, errs
}

// Load reads and decodes the TOML config at path. A missing file is not an
// error: it returns the zero File so callers fall back to environment and
// defaults. Unrecognised keys are reported as an error alongside the decoded
// File, so the caller can warn yet still use the keys it understood; a syntax
// error returns the zero File and the error.
func Load(path string) (File, error) {
	var f File
	md, err := toml.DecodeFile(path, &f)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return f, fmt.Errorf("unrecognised config keys: %s", joinKeys(undecoded))
	}
	return f, nil
}

// Resolve merges the file with the environment over the built-in defaults,
// applying the precedence environment variable > config file > default for each
// setting. lookup is the os.LookupEnv signature; its second result distinguishes
// an unset variable from one set to an empty or "false" value, so an environment
// variable can override a file value in either direction. Non-fatal parse
// problems (a malformed duration) are returned for the caller to log; the
// affected setting falls back to its default.
func Resolve(file File, lookup func(string) (string, bool)) (Settings, []error) {
	var errs []error
	var s Settings

	lifetime, err := KeyLifetime(coalesce(lookup, "SSHAKKU_KEY_LIFETIME", file.KeyLifetime))
	if err != nil {
		errs = append(errs, err)
	}
	s.KeyLifetime = lifetime

	ttl, err := GiveupTTL(coalesce(lookup, "SSHAKKU_GIVEUP_TTL", file.GiveupTTL))
	if err != nil {
		errs = append(errs, err)
	}
	s.GiveupTTL = ttl

	s.MaxAttempts = resolveMaxAttempts(lookup, file.MaxAttempts)
	s.NoGiveup = resolveBool(lookup, "SSHAKKU_NO_GIVEUP", file.NoGiveup)
	s.Quiet = resolveBool(lookup, "SSHAKKU_QUIET", file.Quiet)

	mode, err := resolveWalletStoreMode(file.WalletStoreMode)
	if err != nil {
		errs = append(errs, err)
	}
	s.WalletStoreMode = mode
	s.WalletStoreInclude = file.WalletStoreInclude
	s.WalletStoreExclude = file.WalletStoreExclude

	autoLoadMode, err := resolveAutoLoadMode(file.AutoLoadMode)
	if err != nil {
		errs = append(errs, err)
	}
	s.AutoLoadMode = autoLoadMode
	s.AutoLoadInclude = file.AutoLoadInclude
	s.AutoLoadExclude = file.AutoLoadExclude

	backend, err := resolveSecretBackend(file.SecretBackend)
	if err != nil {
		errs = append(errs, err)
	}
	s.SecretBackend = backend
	s.OnePasswordVault = derefString(file.OnePasswordVault)
	s.BitwardenEmail = derefString(file.BitwardenEmail)
	s.BitwardenServer = derefString(file.BitwardenServer)

	return s, errs
}

// resolveWalletStoreMode is config-file only (no environment override, per
// File's doc comment). An absent or empty value defaults to "all"; an
// unrecognised value falls back to "all" too, reported as an error to log.
func resolveWalletStoreMode(fileVal *string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return WalletStoreModeAll, nil
	}
	switch *fileVal {
	case WalletStoreModeAll, WalletStoreModeInclude, WalletStoreModeExclude:
		return *fileVal, nil
	default:
		return WalletStoreModeAll, fmt.Errorf("invalid wallet_store_mode %q, using %q", *fileVal, WalletStoreModeAll)
	}
}

// resolveAutoLoadMode is config-file only (no environment override, per File's
// doc comment). An absent or empty value defaults to "all"; an unrecognised
// value falls back to "all" too, reported as an error to log.
func resolveAutoLoadMode(fileVal *string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return AutoLoadModeAll, nil
	}
	switch *fileVal {
	case AutoLoadModeAll, AutoLoadModeInclude, AutoLoadModeExclude:
		return *fileVal, nil
	default:
		return AutoLoadModeAll, fmt.Errorf("invalid auto_load_mode %q, using %q", *fileVal, AutoLoadModeAll)
	}
}

// resolveSecretBackend is config-file only (no environment override, per
// File's doc comment: the account identity fields it's paired with don't fit
// an env var cleanly either). An absent or empty value defaults to
// SecretBackendSecretService (today's only behaviour); an unrecognised value
// falls back to the same default, reported as an error to log.
func resolveSecretBackend(fileVal *string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return SecretBackendSecretService, nil
	}
	switch *fileVal {
	case SecretBackendSecretService, SecretBackendOnePassword, SecretBackendBitwarden, SecretBackendKeychain:
		return *fileVal, nil
	default:
		return SecretBackendSecretService, fmt.Errorf("invalid secret_backend %q, using %q", *fileVal, SecretBackendSecretService)
	}
}

// derefString returns "" for a nil pointer, else the pointed-to value.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// coalesce returns the environment value when the variable is set, else the file
// value when present, else "" (which the duration parsers map to the default).
func coalesce(lookup func(string) (string, bool), key string, fileVal *string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	if fileVal != nil {
		return *fileVal
	}
	return ""
}

// resolveMaxAttempts applies env > file > 0 (the loader's own default). A
// set-but-invalid environment value falls through to the file then the default.
func resolveMaxAttempts(lookup func(string) (string, bool), fileVal *int) int {
	if v, ok := lookup("SSHAKKU_MAX_ATTEMPTS"); ok {
		if n := EnvInt(v); n > 0 {
			return n
		}
	}
	if fileVal != nil && *fileVal > 0 {
		return *fileVal
	}
	return 0
}

// resolveBool applies env > file > false. A set environment variable wins in
// either direction (e.g. SSHAKKU_QUIET=0 overrides quiet = true in the file).
func resolveBool(lookup func(string) (string, bool), key string, fileVal *bool) bool {
	if v, ok := lookup(key); ok {
		return IsTruthy(v)
	}
	if fileVal != nil {
		return *fileVal
	}
	return false
}

// joinKeys renders TOML key paths for an error message.
func joinKeys(keys []toml.Key) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.String()
	}
	return strings.Join(names, ", ")
}
