package config

import (
	"github.com/OrbintSoft/sshakku/internal/keys"
	"testing"
	"time"
)

func TestKeyLifetime(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{"empty defaults", "", DefaultKeyLifetime, false},
		{"explicit hours", "1h", time.Hour, false},
		{"minutes", "20m", 20 * time.Minute, false},
		{"zero disables", "0", 0, false},
		{"negative disables", "-5m", 0, false},
		{"malformed falls back", "banana", DefaultKeyLifetime, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := KeyLifetime(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("KeyLifetime(%q) err = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("KeyLifetime(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestGiveupTTL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{"empty defaults", "", DefaultGiveupTTL, false},
		{"explicit hours", "2h", 2 * time.Hour, false},
		{"zero never expires", "0", 0, false},
		{"negative never expires", "-1h", 0, false},
		{"malformed falls back", "soon", DefaultGiveupTTL, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GiveupTTL(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GiveupTTL(%q) err = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GiveupTTL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", 0},
		{" 5 ", 5},
		{"3", 3},
		{"0", 0},
		{"-2", 0},
		{"banana", 0},
	}
	for _, tc := range tests {
		if got := EnvInt(tc.raw); got != tc.want {
			t.Errorf("EnvInt(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestIsTruthy(t *testing.T) {
	truthy := []string{"1", "true", "yes", "on", "TRUE", " On "}
	for _, raw := range truthy {
		if !IsTruthy(raw) {
			t.Errorf("IsTruthy(%q) = false, want true", raw)
		}
	}
	falsy := []string{"", "0", "false", "no", "off", "banana"}
	for _, raw := range falsy {
		if IsTruthy(raw) {
			t.Errorf("IsTruthy(%q) = true, want false", raw)
		}
	}
}

// TestCommandTimeout covers the rule that separates these budgets from every
// other duration in the configuration: there is no way to ask for "no limit".
// A key lifetime of 0 sensibly means "never expire"; a command timeout of 0
// would mean a wallet or a dialog could hold a shell for as long as it liked,
// which is the failure these settings exist to prevent — so it is rejected and
// the default stands.
func TestCommandTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{"empty takes the default", "", keys.DefaultCommandTimeout, false},
		{"a duration is honoured", "3s", 3 * time.Second, false},
		{"zero is refused", "0s", keys.DefaultCommandTimeout, true},
		{"negative is refused", "-5s", keys.DefaultCommandTimeout, true},
		{"malformed is refused", "soon", keys.DefaultCommandTimeout, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CommandTimeout(tc.raw)
			if got != tc.want {
				t.Errorf("CommandTimeout(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("CommandTimeout(%q) error = %v, want error: %v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

func TestInteractiveTimeout(t *testing.T) {
	if got, err := InteractiveTimeout(""); got != keys.DefaultInteractiveTimeout || err != nil {
		t.Errorf("InteractiveTimeout(\"\") = %v, %v, want %v, nil", got, err, keys.DefaultInteractiveTimeout)
	}
	if got, err := InteractiveTimeout("90s"); got != 90*time.Second || err != nil {
		t.Errorf("InteractiveTimeout(\"90s\") = %v, %v, want 90s, nil", got, err)
	}
	if got, err := InteractiveTimeout("0"); got != keys.DefaultInteractiveTimeout || err == nil {
		t.Errorf("InteractiveTimeout(\"0\") = %v, %v, want the default and an error", got, err)
	}
}
