package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"

	"github.com/OrbintSoft/sshakku/internal/keys"
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
	CommandTimeout     *string `toml:"command_timeout"`
	InteractiveTimeout *string `toml:"interactive_timeout"`

	// ServicePrefix names sshakku's own entries in whatever store is in use,
	// and is config-file only for a reason of its own: it decides where
	// passphrases live, so a variable exported in one shell and not the next
	// would have sshakku save under one name and look under another.
	ServicePrefix *string `toml:"service_prefix"`

	// SecretContainer names the compartment SSHakku makes for itself in the
	// store — a Secret Service collection, a KeePassXC group — and is
	// config-file only for the same reason ServicePrefix is.
	SecretContainer *string `toml:"secret_container"`

	// KeyDir and KeyPatterns say which files SSHakku treats as the user's
	// keys: where to look, and which names there are keys. Both are
	// config-file only — a list of patterns does not fit one environment
	// variable, and the two halves of one rule are better read from one place
	// than from two that can disagree.
	KeyDir      *string  `toml:"key_dir"`
	KeyPatterns []string `toml:"key_patterns"`

	SecretBackend    *string `toml:"secret_backend"`
	OnePasswordVault *string `toml:"onepassword_vault"`
	BitwardenEmail   *string `toml:"bitwarden_email"`
	BitwardenServer  *string `toml:"bitwarden_server"`

	// KeePassXCRoute pins how KeePassXC is reached. Absent, SSHakku picks per
	// platform; set, the named route is used and no other, so a route that is
	// unavailable is reported rather than quietly swapped for another.
	KeePassXCRoute    *string `toml:"keepassxc_route"`
	KeePassXCDatabase *string `toml:"keepassxc_database"`
	KeePassXCKeyFile  *string `toml:"keepassxc_key_file"`

	// GUIPrompter names the dialog a passphrase is asked for in. It is
	// config-file only for the reason the wallet choice is: it decides how the
	// user is spoken to, and a variable exported in one shell and not the next
	// would ask two different ways for no reason the user could see.
	GUIPrompter *string `toml:"gui_prompter"`

	// OnDismiss is what closing a passphrase prompt without answering means for
	// the keys that come after it. Config-file only for the same reason as the
	// dialog it answers: it decides how the user is spoken to.
	OnDismiss *string `toml:"on_dismiss"`
}

// Settings is the configuration resolved from environment, file, and defaults.
type Settings struct {
	KeyLifetime time.Duration // 0 disables agent key expiry
	MaxAttempts int           // 0 lets the loader use its own default
	GiveupTTL   time.Duration // 0 never expires the give-up record
	NoGiveup    bool          // true disables give-up tracking entirely

	// CommandTimeout and InteractiveTimeout bound every external command
	// SSHakku runs: the first for one that should answer on its own, the second
	// for one waiting on a person. Both are always positive — there is no
	// setting for "wait forever".
	CommandTimeout     time.Duration
	InteractiveTimeout time.Duration
	Quiet              bool // true suppresses the failure notice

	// WalletStoreMode is one of "all", "include", or "exclude"; see StoresWallet.
	WalletStoreMode    string
	WalletStoreInclude []string // key names consulted only when mode is "include"
	WalletStoreExclude []string // key names consulted only when mode is "exclude"

	// AutoLoadMode is one of "all", "include", or "exclude"; see AutoLoads.
	AutoLoadMode    string
	AutoLoadInclude []string // key names consulted only when mode is "include"
	AutoLoadExclude []string // key names consulted only when mode is "exclude"

	// ServicePrefix is the name SSHakku's entries carry in the secret store,
	// ahead of the key's own name. It is always set: what a store writes, what
	// a lookup reads back and what `forget` deletes are all built from it, and
	// leaving it empty for one of them to fill in is how they come to disagree.
	ServicePrefix string

	// SecretContainer is the compartment SSHakku keeps its entries in, where the
	// store has room for one. Unlike ServicePrefix it is empty when the user has
	// named none: there is no one default to put here, since the collection and
	// the group are called different things, so each backend supplies its own.
	SecretContainer string

	// KeyDir and KeyPatterns are what the user asked for, unresolved: KeyDir
	// as written (possibly relative to home, which this layer does not know)
	// and KeyPatterns nil whenever SSHakku's own rule applies. KeyEnumerator
	// turns the pair into the enumerator every caller uses.
	KeyDir      string
	KeyPatterns []string

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

	// KeePassXCRoute is how KeePassXC is reached; one of the KeePassXCRoute*
	// constants. Anything but "auto" is exclusive: that route is used and no
	// other, and its being unavailable is reported rather than worked around.
	// Consulted only when SecretBackend is SecretBackendKeePassXC.
	KeePassXCRoute string
	// KeePassXCDatabase and KeePassXCKeyFile locate the database file, needed
	// only by the CLI route — the other routes talk to a KeePassXC that has
	// already opened it.
	KeePassXCDatabase string
	KeePassXCKeyFile  string

	// GUIPrompter is the dialog to ask in; one of the GUIPrompter* constants.
	// "auto" lets SSHakku use whichever the session has, "none" refuses a
	// dialog outright, and any other value names one and only that one — a
	// prompter that cannot run is never swapped for a different dialog.
	GUIPrompter string

	// OnDismiss is what a dismissed passphrase prompt means for the keys still
	// to come; one of the keys.OnDismiss* values, and never empty.
	OnDismiss string
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

// Secret backend choices for Settings.SecretBackend that any system can offer,
// because they are programs a user installs rather than something the operating
// system either has or has not.
//
// The ones that are the operating system's own — the freedesktop Secret
// Service, the OS keychain — are declared beside the platform that has them, in
// the per-platform backends_*.go, along with the list of what can be chosen
// here and which is chosen by default.
const (
	SecretBackendOnePassword = "1password"
	SecretBackendBitwarden   = "bitwarden"
	SecretBackendKeePassXC   = "keepassxc"
)

// KeePassXC routes for Settings.KeePassXCRoute. The wallet is named by
// secret_backend; this says how to reach it. A route is available wherever it
// can work rather than on one platform only — the exception is the Secret
// Service, which no macOS system provides.
const (
	// KeePassXCRouteAuto picks per platform: the Secret Service where there is
	// one, the local protocol otherwise, falling back to the CLI. It is the
	// only value that falls back at all.
	KeePassXCRouteAuto = "auto"
	// KeePassXCRouteSecretService reaches KeePassXC through the freedesktop
	// Secret Service, which KeePassXC implements itself.
	KeePassXCRouteSecretService = "secret-service"
	// KeePassXCRouteNative speaks KeePassXC's local socket protocol to a
	// running, unlocked instance.
	KeePassXCRouteNative = "native"
	// KeePassXCRouteCLI runs keepassxc-cli against the database file.
	KeePassXCRouteCLI = "cli"
)

// prompt.Prompter choices for Settings.GUIPrompter that mean the same thing on every
// system, because they name no program at all. The dialogs themselves are
// declared beside the platform that can have them, in the per-platform
// prompters_*.go, along with the list of what can be chosen here.
const (
	// GUIPrompterAuto uses whichever dialog the session turns out to have.
	GUIPrompterAuto = "auto"
	// GUIPrompterNone refuses a dialog: the passphrase is asked for on the
	// terminal even where there is a screen to show one on.
	GUIPrompterNone = "none"
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

// KeyEnumerator turns the discovery settings into the enumerator every caller
// reads its keys through, given the home directory a relative KeyDir is
// resolved against. It exists so there is one such mapping: what SSHakku loads
// and what it reports are then the same set by construction, and a report of a
// directory nobody read is the most misleading thing a diagnostic can say.
func (s Settings) KeyEnumerator(home string) keys.Enumerator {
	if s.KeyDir == "" {
		return keys.Enumerator{
			Dir:      filepath.Join(home, keys.DefaultKeyDirName),
			Patterns: s.KeyPatterns,
		}
	}
	// A directory the user named must be there: it is the only way to tell a
	// mistyped one from one that simply holds no keys, since both produce the
	// same empty agent and the same silence.
	return keys.Enumerator{
		Dir:       resolveHomePath(home, s.KeyDir),
		Patterns:  s.KeyPatterns,
		MustExist: true,
	}
}

// resolveHomePath reads a directory as a person writes one in a config file:
// absolute where it starts at the root, and otherwise relative to home, whether
// or not they spelled that "~/".
func resolveHomePath(home, dir string) string {
	switch {
	case dir == "~":
		return home
	case strings.HasPrefix(dir, "~/"):
		dir = strings.TrimPrefix(dir, "~/")
	case filepath.IsAbs(dir):
		return filepath.Clean(dir)
	}
	return filepath.Join(home, dir)
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

	if other.CommandTimeout != nil {
		merged.CommandTimeout = other.CommandTimeout
	}
	if other.InteractiveTimeout != nil {
		merged.InteractiveTimeout = other.InteractiveTimeout
	}

	if other.ServicePrefix != nil {
		merged.ServicePrefix = other.ServicePrefix
	}
	if other.SecretContainer != nil {
		merged.SecretContainer = other.SecretContainer
	}
	if other.KeyDir != nil {
		merged.KeyDir = other.KeyDir
	}
	if other.KeyPatterns != nil {
		merged.KeyPatterns = other.KeyPatterns
	}
	if other.GUIPrompter != nil {
		merged.GUIPrompter = other.GUIPrompter
	}
	if other.OnDismiss != nil {
		merged.OnDismiss = other.OnDismiss
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
	if other.KeePassXCRoute != nil {
		merged.KeePassXCRoute = other.KeePassXCRoute
	}
	if other.KeePassXCDatabase != nil {
		merged.KeePassXCDatabase = other.KeePassXCDatabase
	}
	if other.KeePassXCKeyFile != nil {
		merged.KeePassXCKeyFile = other.KeePassXCKeyFile
	}

	return merged
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

// SettingError is a value SSHakku would not use, tied to the setting it was
// written for. The message is the underlying one unchanged, so a caller that
// only logs it sees exactly what it saw before; a caller reporting settings one
// by one can put the refusal beside the setting it belongs to instead of
// leaving the reader to match them up by eye.
type SettingError struct {
	Key string // the config-file key the refused value was written for
	Err error
}

func (e *SettingError) Error() string { return e.Err.Error() }

func (e *SettingError) Unwrap() error { return e.Err }

// refused appends err to errs as a refusal of key, and returns errs unchanged
// when there was no error.
func refused(errs []error, key string, err error) []error {
	if err == nil {
		return errs
	}
	return append(errs, &SettingError{Key: key, Err: err})
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
	errs = refused(errs, "key_lifetime", err)
	s.KeyLifetime = lifetime

	ttl, err := GiveupTTL(coalesce(lookup, "SSHAKKU_GIVEUP_TTL", file.GiveupTTL))
	errs = refused(errs, "giveup_ttl", err)
	s.GiveupTTL = ttl

	cmdTimeout, err := CommandTimeout(coalesce(lookup, "SSHAKKU_COMMAND_TIMEOUT", file.CommandTimeout))
	errs = refused(errs, "command_timeout", err)
	s.CommandTimeout = cmdTimeout

	interactive, err := InteractiveTimeout(coalesce(lookup, "SSHAKKU_INTERACTIVE_TIMEOUT", file.InteractiveTimeout))
	errs = refused(errs, "interactive_timeout", err)
	s.InteractiveTimeout = interactive

	s.MaxAttempts = resolveMaxAttempts(lookup, file.MaxAttempts)
	s.NoGiveup = resolveBool(lookup, "SSHAKKU_NO_GIVEUP", file.NoGiveup)
	s.Quiet = resolveBool(lookup, "SSHAKKU_QUIET", file.Quiet)

	mode, err := resolveWalletStoreMode(file.WalletStoreMode)
	errs = refused(errs, "wallet_store_mode", err)
	s.WalletStoreMode = mode
	s.WalletStoreInclude = file.WalletStoreInclude
	s.WalletStoreExclude = file.WalletStoreExclude

	autoLoadMode, err := resolveAutoLoadMode(file.AutoLoadMode)
	errs = refused(errs, "auto_load_mode", err)
	s.AutoLoadMode = autoLoadMode
	s.AutoLoadInclude = file.AutoLoadInclude
	s.AutoLoadExclude = file.AutoLoadExclude

	prefix, err := resolveServicePrefix(file.ServicePrefix)
	errs = refused(errs, "service_prefix", err)
	s.ServicePrefix = prefix

	container, err := resolveSecretContainer(file.SecretContainer)
	errs = refused(errs, "secret_container", err)
	s.SecretContainer = container

	s.KeyDir = derefString(file.KeyDir)
	patterns, err := resolveKeyPatterns(file.KeyPatterns)
	errs = refused(errs, "key_patterns", err)
	s.KeyPatterns = patterns

	backend, err := resolveSecretBackend(file.SecretBackend)
	errs = refused(errs, "secret_backend", err)
	s.SecretBackend = backend
	s.OnePasswordVault = derefString(file.OnePasswordVault)
	s.BitwardenEmail = derefString(file.BitwardenEmail)
	s.BitwardenServer = derefString(file.BitwardenServer)

	route, err := resolveKeePassXCRoute(file.KeePassXCRoute)
	errs = refused(errs, "keepassxc_route", err)
	s.KeePassXCRoute = route
	s.KeePassXCDatabase = derefString(file.KeePassXCDatabase)
	s.KeePassXCKeyFile = derefString(file.KeePassXCKeyFile)

	prompter, err := resolveGUIPrompter(file.GUIPrompter)
	errs = refused(errs, "gui_prompter", err)
	s.GUIPrompter = prompter

	dismiss, err := resolveOnDismiss(file.OnDismiss)
	errs = refused(errs, "on_dismiss", err)
	s.OnDismiss = dismiss

	return s, errs
}

// resolveServicePrefix is config-file only (per File's doc comment). An absent
// or empty value takes the default; a value carrying whitespace or a slash is
// refused and the default used instead, reported as an error to log — a slash
// reads as a folder separator to some wallets, and whitespace makes an entry
// name that its store's own tools cannot easily be pointed at.
//
// Nothing here judges whether a prefix is distinctive enough to be sshakku's
// alone, because nothing can: in a store shared with other programs that is
// what keeps `forget --all` off their entries, and it is the person choosing
// the name who knows what else lives there. CONFIGURATION.md says so.
func resolveServicePrefix(fileVal *string) (string, error) {
	prefix := derefString(fileVal)
	switch {
	case prefix == "":
		return keys.DefaultServicePrefix, nil
	case unusableName(prefix):
		return keys.DefaultServicePrefix, fmt.Errorf("invalid service_prefix %q: whitespace and %q are not allowed, using %q", prefix, "/", keys.DefaultServicePrefix)
	}
	return prefix, nil
}

// unusableName reports whether a name a user chose for SSHakku's entries, or
// for the compartment holding them, is one no store can be relied on to hold
// verbatim: whitespace makes a name the store's own tools cannot easily be
// pointed at, and a slash reads as a folder separator to some of them — a
// KeePassXC group is addressed by path.
func unusableName(name string) bool {
	return strings.ContainsFunc(name, unicode.IsSpace) || strings.Contains(name, "/")
}

// desktopOwnWallets are the names a desktop uses for the wallet it keeps for
// itself. Resolving a collection by name can adopt an existing one rather than
// create it, and SSHakku empties its own compartment without reading whose
// entry is whose, so a compartment named any of these would put every password
// the desktop holds within reach of `sshakku forget --all`.
//
// It is the list SSHakku knows to refuse, not every name some wallet somewhere
// answers to; CONFIGURATION.md says so, and says to pick a name nothing else
// has taken.
var desktopOwnWallets = []string{"default", "login", "session", "kdewallet"}

// resolveSecretContainer is config-file only (per File's doc comment). Unlike
// resolveServicePrefix it returns the empty string for an absent value rather
// than a default: the collection and the group SSHakku makes for itself are
// called different things, and each backend fills its own in. A refused value
// resolves the same way, so a name SSHakku will not take leaves the entries
// where they already are instead of somewhere else again.
func resolveSecretContainer(fileVal *string) (string, error) {
	container := derefString(fileVal)
	switch {
	case container == "":
		return "", nil
	case unusableName(container):
		return "", fmt.Errorf("invalid secret_container %q: whitespace and %q are not allowed, using SSHakku's own", container, "/")
	case slices.Contains(desktopOwnWallets, strings.ToLower(container)):
		return "", fmt.Errorf("invalid secret_container %q: that name belongs to your desktop's own wallet, whose contents are not SSHakku's to delete; using SSHakku's own", container)
	}
	return container, nil
}

// resolveKeyPatterns is config-file only (per File's doc comment). An absent
// list leaves SSHakku's own naming rule in force, and so does a list it cannot
// use: a pattern is refused when it can never match a file name — it is empty,
// it holds a path separator, or it is malformed — and the whole list goes with
// it. Keeping the readable half would enforce a rule the user did not write,
// and obeying an empty list would leave them with no keys at all, which is the
// failure the setting exists to remove.
func resolveKeyPatterns(fileVal []string) ([]string, error) {
	if fileVal == nil {
		return nil, nil
	}
	if len(fileVal) == 0 {
		return nil, errors.New("invalid key_patterns: the list is empty, using SSHakku's own naming rule")
	}
	for _, pattern := range fileVal {
		switch _, err := filepath.Match(pattern, "name"); {
		case pattern == "" || strings.Contains(pattern, "/"):
			return nil, fmt.Errorf("invalid key_patterns entry %q: a pattern matches a file name, not a path, using SSHakku's own naming rule", pattern)
		case err != nil:
			return nil, fmt.Errorf("invalid key_patterns entry %q: %w, using SSHakku's own naming rule", pattern, err)
		}
	}
	return slices.Clone(fileVal), nil
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

// SecretBackends returns the wallets that can be chosen on this system, in the
// order a message to the user should list them. It is the one list: a caller
// that offers the user a choice, or rejects one, asks here rather than keeping
// a copy that has to be remembered whenever this one changes.
func SecretBackends() []string {
	return append([]string(nil), platformSecretBackends...)
}

// DefaultSecretBackend is the wallet used when the configuration names none:
// the one the operating system provides itself.
func DefaultSecretBackend() string { return platformDefaultSecretBackend }

// SecretBackendAvailable reports whether name is a wallet this system has.
func SecretBackendAvailable(name string) bool {
	for _, available := range platformSecretBackends {
		if name == available {
			return true
		}
	}
	return false
}

// resolveSecretBackend is config-file only (no environment override, per
// File's doc comment: the account identity fields it's paired with don't fit
// an env var cleanly either). It answers against the wallets this system
// actually has.
func resolveSecretBackend(fileVal *string) (string, error) {
	return resolveSecretBackendFrom(fileVal, platformSecretBackends, platformDefaultSecretBackend)
}

// resolveSecretBackendFrom is the choice itself, with the wallets on offer and
// the one to fall back on taken as arguments so every platform's answer can be
// checked from any of them — the tables are per-platform, the rule is not.
//
// An absent or empty value takes the fallback. Anything else that is not on
// offer takes it too and is reported: a name meant for another operating
// system is a mistake in the configuration, not a wallet missing a piece the
// user could install, and the two deserve different answers. Falling back
// rather than failing keeps a bad line in a config file from taking the login
// shell down with it.
func resolveSecretBackendFrom(fileVal *string, available []string, fallback string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return fallback, nil
	}
	for _, name := range available {
		if *fileVal == name {
			return *fileVal, nil
		}
	}
	return fallback, fmt.Errorf("secret_backend %q is not a wallet this system has, using %q", *fileVal, fallback)
}

// resolveGUIPrompter picks the dialog to ask in.
func resolveGUIPrompter(fileVal *string) (string, error) {
	return resolveGUIPrompterFrom(fileVal, platformGUIPrompters)
}

// resolveGUIPrompterFrom is the choice itself, with the dialogs this system can
// have taken as an argument so every platform's answer can be checked from any
// of them — the table is per-platform, the rule is not.
//
// An absent or empty value means "auto". A name no dialog here answers to is a
// mistake in the configuration rather than a dialog waiting to be installed —
// naming macOS's on Linux can never come true — so it is reported and "auto"
// applies, which leaves the user asked rather than unasked.
func resolveGUIPrompterFrom(fileVal *string, available []string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return GUIPrompterAuto, nil
	}
	for _, name := range available {
		if *fileVal == name {
			return *fileVal, nil
		}
	}
	return GUIPrompterAuto, fmt.Errorf("gui_prompter %q is not a dialog this system has, using %q", *fileVal, GUIPrompterAuto)
}

// resolveOnDismiss picks what closing a passphrase prompt without answering
// means for the keys that come after it. An absent or empty value ends the
// asking, since a window nobody asked for should take one gesture to be rid of,
// not one per key. An unrecognised value is a mistake in the configuration
// rather than a behaviour waiting to exist, so it is reported and the default
// applies — which asks less rather than more.
func resolveOnDismiss(fileVal *string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return keys.OnDismissStop, nil
	}
	switch *fileVal {
	case keys.OnDismissStop, keys.OnDismissSkip, keys.OnDismissRetry:
		return *fileVal, nil
	default:
		return keys.OnDismissStop, fmt.Errorf("invalid on_dismiss %q, using %q", *fileVal, keys.OnDismissStop)
	}
}

// resolveKeePassXCRoute is config-file only, like the backend choice it
// refines. An absent or empty value means "auto"; an unrecognised one falls
// back to "auto" and is reported, so a typo does not silently pin a route the
// user did not name.
func resolveKeePassXCRoute(fileVal *string) (string, error) {
	if fileVal == nil || *fileVal == "" {
		return KeePassXCRouteAuto, nil
	}
	switch *fileVal {
	case KeePassXCRouteAuto, KeePassXCRouteSecretService, KeePassXCRouteNative, KeePassXCRouteCLI:
		return *fileVal, nil
	default:
		return KeePassXCRouteAuto, fmt.Errorf("invalid keepassxc_route %q, using %q", *fileVal, KeePassXCRouteAuto)
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
