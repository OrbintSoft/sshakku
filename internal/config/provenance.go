package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// OriginKind says what sort of thing put a value in force.
type OriginKind int

const (
	// OriginDefault is SSHakku's own value, applying because nobody chose one.
	OriginDefault OriginKind = iota
	// OriginFile is a configuration file, named by Origin.Name.
	OriginFile
	// OriginEnv is an environment variable, named by Origin.Name.
	OriginEnv
)

// Origin names what put a setting's value in force.
type Origin struct {
	Kind OriginKind
	Name string // the file's path or the variable's name; empty for a default
}

// Refusal is a value SSHakku would not use, and where it was written. The
// setting it belongs to falls back to the default, so without this a report
// would show a value the user never wrote and no sign of the one they did.
type Refusal struct {
	From Origin
	Err  error
}

// Setting is one line of the answer to "what is SSHakku configured to do":
// the key as it is written in a config file, the value actually in force, what
// put it there, and the value that was refused if one was.
type Setting struct {
	Key     string
	Value   string
	From    Origin
	Refused *Refusal
}

// Explain resolves sources against the environment and reports every setting
// with the value in force and where that value came from.
//
// It resolves exactly what a login shell resolves, from the same sources in the
// same order: a report assembled any other way describes a configuration
// nothing acts on.
func Explain(sources []Source, lookup func(string) (string, bool)) []Setting {
	settings, errs := Resolve(Merged(sources), lookup)

	report := make([]Setting, 0, len(settingTable))
	for _, desc := range settingTable {
		s := Setting{Key: desc.key, Value: desc.value(settings)}
		stated := desc.statedBy(sources, lookup)
		if err := refusalOf(errs, desc.key); err != nil {
			// The value in force is the built-in one: what the user wrote was
			// not used, and saying it came from their file would be the lie
			// this report exists to remove.
			s.From = Origin{Kind: OriginDefault}
			s.Refused = &Refusal{From: stated, Err: err}
		} else {
			s.From = stated
		}
		report = append(report, s)
	}
	return report
}

// Overrule is a setting one file states and something else decides: the value
// written there is not the one in force, and that file is the one place where
// nothing can say so.
type Overrule struct {
	Key string
	By  Origin
}

// Overruled reports the settings the file at path states that something later
// overrules — a file applied after it, or an exported variable. Editing a
// value somebody else decides is an edit with no effect, which is worth
// hearing while the person who made it is still there.
func Overruled(sources []Source, path string, lookup func(string) (string, bool)) []Overrule {
	var overruled []Overrule
	for _, desc := range settingTable {
		if !states(sources, path, desc) {
			continue
		}
		from := desc.statedBy(sources, lookup)
		if from.Kind == OriginFile && from.Name == path {
			continue
		}
		overruled = append(overruled, Overrule{Key: desc.key, By: from})
	}
	return overruled
}

// states reports whether the source at path sets the setting at all.
func states(sources []Source, path string, desc settingDesc) bool {
	for _, s := range sources {
		if s.Path == path {
			return desc.set(s.File)
		}
	}
	return false
}

// refusalOf returns the error reported against key, if any.
func refusalOf(errs []error, key string) error {
	for _, err := range errs {
		var setting *SettingError
		if errors.As(err, &setting) && setting.Key == key {
			return err
		}
	}
	return nil
}

// settingDesc is one row of the table behind the report: how a setting is
// written, where it can be written, and what its value looks like once
// resolved. The rows are hand-written and a test walks File's toml tags to
// catch the setting that arrives without one.
type settingDesc struct {
	key string
	// env is the variable that can override the file, or "" for a setting that
	// can only be written in a file.
	env string
	// envUsed reports whether a value found in env is one Resolve will act on.
	// Nil means any value is; max_attempts is the exception, silently ignoring
	// what it cannot use, and attributing the value in force to a variable that
	// was disregarded would send the user to edit the wrong thing.
	envUsed func(string) bool
	// set reports whether this file states the setting at all — the question
	// Merge answers by keeping the value and discarding where it came from.
	set func(File) bool
	// value renders what is in force, which for a setting nobody wrote is the
	// default SSHakku applies rather than the empty value it stores.
	value func(Settings) string
}

// statedBy returns where the setting's value was stated: the environment when
// a variable holds one, else the last file to set it, else nowhere.
func (d settingDesc) statedBy(sources []Source, lookup func(string) (string, bool)) Origin {
	if d.env != "" {
		if v, ok := lookup(d.env); ok && (d.envUsed == nil || d.envUsed(v)) {
			return Origin{Kind: OriginEnv, Name: d.env}
		}
	}
	for i := len(sources) - 1; i >= 0; i-- {
		if d.set(sources[i].File) {
			return Origin{Kind: OriginFile, Name: sources[i].Path}
		}
	}
	return Origin{Kind: OriginDefault}
}

// settingTable lists every setting a user can write, in the order the report
// prints them: the ones every account meets first, then the wallet, then the
// keys, then the per-backend account details.
var settingTable = []settingDesc{
	{
		key: "key_lifetime", env: "SSHAKKU_KEY_LIFETIME",
		set:   func(f File) bool { return f.KeyLifetime != nil },
		value: func(s Settings) string { return duration(s.KeyLifetime, "no expiry") },
	},
	{
		key: "max_attempts", env: "SSHAKKU_MAX_ATTEMPTS",
		envUsed: func(v string) bool { return EnvInt(v) > 0 },
		set:     func(f File) bool { return f.MaxAttempts != nil },
		value: func(s Settings) string {
			if s.MaxAttempts < 1 {
				return strconv.Itoa(keys.DefaultMaxAttempts)
			}
			return strconv.Itoa(s.MaxAttempts)
		},
	},
	{
		key: "giveup_ttl", env: "SSHAKKU_GIVEUP_TTL",
		set:   func(f File) bool { return f.GiveupTTL != nil },
		value: func(s Settings) string { return duration(s.GiveupTTL, "never expires") },
	},
	{
		key: "no_giveup", env: "SSHAKKU_NO_GIVEUP",
		set:   func(f File) bool { return f.NoGiveup != nil },
		value: func(s Settings) string { return strconv.FormatBool(s.NoGiveup) },
	},
	{
		key: "quiet", env: "SSHAKKU_QUIET",
		set:   func(f File) bool { return f.Quiet != nil },
		value: func(s Settings) string { return strconv.FormatBool(s.Quiet) },
	},
	{
		key: "command_timeout", env: "SSHAKKU_COMMAND_TIMEOUT",
		set:   func(f File) bool { return f.CommandTimeout != nil },
		value: func(s Settings) string { return s.CommandTimeout.String() },
	},
	{
		key: "interactive_timeout", env: "SSHAKKU_INTERACTIVE_TIMEOUT",
		set:   func(f File) bool { return f.InteractiveTimeout != nil },
		value: func(s Settings) string { return s.InteractiveTimeout.String() },
	},
	{
		key:   "secret_backend",
		set:   func(f File) bool { return f.SecretBackend != nil },
		value: func(s Settings) string { return s.SecretBackend },
	},
	{
		key:   "service_prefix",
		set:   func(f File) bool { return f.ServicePrefix != nil },
		value: func(s Settings) string { return s.ServicePrefix },
	},
	{
		key: "secret_container",
		set: func(f File) bool { return f.SecretContainer != nil },
		// Empty is not "nothing": each backend names the compartment it makes
		// for itself, so there is no one value to print here.
		value: func(s Settings) string { return orElse(s.SecretContainer, "(the wallet's own)") },
	},
	{
		key: "key_dir",
		set: func(f File) bool { return f.KeyDir != nil },
		value: func(s Settings) string {
			return orElse(s.KeyDir, filepath.Join("~", keys.DefaultKeyDirName))
		},
	},
	{
		key: "key_patterns",
		set: func(f File) bool { return f.KeyPatterns != nil },
		value: func(s Settings) string {
			if len(s.KeyPatterns) == 0 {
				return list(keys.DefaultKeyPatterns())
			}
			return list(s.KeyPatterns)
		},
	},
	{
		key:   "wallet_store_mode",
		set:   func(f File) bool { return f.WalletStoreMode != nil },
		value: func(s Settings) string { return s.WalletStoreMode },
	},
	{
		key:   "wallet_store_include",
		set:   func(f File) bool { return f.WalletStoreInclude != nil },
		value: func(s Settings) string { return list(s.WalletStoreInclude) },
	},
	{
		key:   "wallet_store_exclude",
		set:   func(f File) bool { return f.WalletStoreExclude != nil },
		value: func(s Settings) string { return list(s.WalletStoreExclude) },
	},
	{
		key:   "auto_load_mode",
		set:   func(f File) bool { return f.AutoLoadMode != nil },
		value: func(s Settings) string { return s.AutoLoadMode },
	},
	{
		key:   "auto_load_include",
		set:   func(f File) bool { return f.AutoLoadInclude != nil },
		value: func(s Settings) string { return list(s.AutoLoadInclude) },
	},
	{
		key:   "auto_load_exclude",
		set:   func(f File) bool { return f.AutoLoadExclude != nil },
		value: func(s Settings) string { return list(s.AutoLoadExclude) },
	},
	{
		key:   "onepassword_vault",
		set:   func(f File) bool { return f.OnePasswordVault != nil },
		value: func(s Settings) string { return orElse(s.OnePasswordVault, "(unset)") },
	},
	{
		key:   "bitwarden_email",
		set:   func(f File) bool { return f.BitwardenEmail != nil },
		value: func(s Settings) string { return orElse(s.BitwardenEmail, "(unset)") },
	},
	{
		key:   "bitwarden_server",
		set:   func(f File) bool { return f.BitwardenServer != nil },
		value: func(s Settings) string { return orElse(s.BitwardenServer, "bitwarden.com") },
	},
	{
		key:   "keepassxc_route",
		set:   func(f File) bool { return f.KeePassXCRoute != nil },
		value: func(s Settings) string { return s.KeePassXCRoute },
	},
	{
		key:   "keepassxc_database",
		set:   func(f File) bool { return f.KeePassXCDatabase != nil },
		value: func(s Settings) string { return orElse(s.KeePassXCDatabase, "(unset)") },
	},
	{
		key:   "keepassxc_key_file",
		set:   func(f File) bool { return f.KeePassXCKeyFile != nil },
		value: func(s Settings) string { return orElse(s.KeePassXCKeyFile, "(unset)") },
	},
	{
		key:   "gui_prompter",
		set:   func(f File) bool { return f.GUIPrompter != nil },
		value: func(s Settings) string { return s.GUIPrompter },
	},
	{
		key:   "on_dismiss",
		set:   func(f File) bool { return f.OnDismiss != nil },
		value: func(s Settings) string { return s.OnDismiss },
	},
}

// duration renders a resolved duration, spelling out what zero means for the
// setting at hand: "0s" alone reads as "immediately" as easily as "never".
func duration(d time.Duration, zeroMeans string) string {
	if d == 0 {
		return fmt.Sprintf("0s (%s)", zeroMeans)
	}
	return d.String()
}

// list renders a list of key names, and an empty one as the words for it.
func list(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

// orElse renders value, or what stands in for it when the user wrote none.
// Where that is a value in its own right (the directory SSHakku looks in, the
// server it talks to) it is printed as such; where there is no value to print,
// the words for it are parenthesised so nobody reads them as one.
func orElse(value, absent string) string {
	if value == "" {
		return absent
	}
	return value
}
