// Package config resolves SSHakku's tunable settings — key lifetime, retry,
// give-up, and notification behaviour. A setting is taken from its environment
// variable when set, otherwise from the TOML config file, otherwise from a
// built-in default.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// DefaultKeyLifetime caps how long an added key stays in the agent before it
// expires and must be re-added from the wallet. A zero or negative configured
// value disables expiry, so the key stays until the agent does.
const DefaultKeyLifetime = 8 * time.Hour

// DefaultGiveupTTL bounds how long a key stays in the give-up state after its
// retries are exhausted, before a later shell tries it again. A zero or negative
// configured value never expires (the record then clears only on a successful
// add or when the runtime dir is wiped).
const DefaultGiveupTTL = time.Hour

// KeyLifetime parses an agent key lifetime expressed as a Go duration (such as
// "1h" or "20m"), defaulting to DefaultKeyLifetime when raw is empty. A zero or
// negative value disables expiry (returned as 0); a malformed value falls back to
// the default and is returned with an error for the caller to log.
func KeyLifetime(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultKeyLifetime, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return DefaultKeyLifetime, fmt.Errorf("invalid key lifetime %q: %w", raw, err)
	}
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

// GiveupTTL parses a give-up TTL expressed as a Go duration, defaulting to
// DefaultGiveupTTL when raw is empty. A zero or negative value means "never
// expire" (returned as 0); a malformed value falls back to the default and is
// returned with an error for the caller to log.
func GiveupTTL(raw string) (time.Duration, error) {
	if raw == "" {
		return DefaultGiveupTTL, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return DefaultGiveupTTL, fmt.Errorf("invalid give-up TTL %q: %w", raw, err)
	}
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

// CommandTimeout parses the budget for something that is not waiting on a
// person — reading the wallet, probing the display — expressed as a Go
// duration, defaulting to keys.DefaultCommandTimeout when raw is empty. Most of
// what it bounds is an external command, but whether something is run or called
// into is not what decides: who is being waited for is.
//
// Unlike a key lifetime, zero and negative do not mean "no limit": something
// with no limit can hold a login shell, or an ssh at a passphrase prompt, for
// as long as it likes. They fall back to the default and are returned with an
// error for the caller to log.
func CommandTimeout(raw string) (time.Duration, error) {
	return positiveDuration(raw, keys.DefaultCommandTimeout, "command timeout")
}

// InteractiveTimeout parses the budget for something that is waiting on a
// person — a password dialog, a CLI deferring to a desktop app for approval, an
// OS keychain that has put up an approval dialog of its own — defaulting to
// keys.DefaultInteractiveTimeout when raw is empty. It is
// generous, since the limit is human, but finite for the same reason as
// CommandTimeout: a dialog nobody answers must not strand the shell.
func InteractiveTimeout(raw string) (time.Duration, error) {
	return positiveDuration(raw, keys.DefaultInteractiveTimeout, "interactive timeout")
}

// positiveDuration parses raw as a Go duration that must be greater than zero,
// falling back to fallback (with an error to log) for anything else.
func positiveDuration(raw string, fallback time.Duration, what string) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback, fmt.Errorf("invalid %s %q: %w", what, raw, err)
	}
	if d <= 0 {
		return fallback, fmt.Errorf("invalid %s %q: must be greater than zero", what, raw)
	}
	return d, nil
}

// EnvInt parses a positive integer from raw, returning 0 (let the consumer use
// its own default) for an empty, malformed, or non-positive value.
func EnvInt(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// IsTruthy reports whether raw is a recognised affirmative value, used for
// boolean opt-out switches.
func IsTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
