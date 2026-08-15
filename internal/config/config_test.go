package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/run"
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
			if tc.wantErr {
				assert.Errorf(t, err, "KeyLifetime(%q) must be reported", tc.raw)
			} else {
				assert.NoErrorf(t, err, "KeyLifetime(%q)", tc.raw)
			}
			assert.Equalf(t, tc.want, got, "KeyLifetime(%q)", tc.raw)
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
			if tc.wantErr {
				assert.Errorf(t, err, "GiveupTTL(%q) must be reported", tc.raw)
			} else {
				assert.NoErrorf(t, err, "GiveupTTL(%q)", tc.raw)
			}
			assert.Equalf(t, tc.want, got, "GiveupTTL(%q)", tc.raw)
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
		assert.Equalf(t, tc.want, EnvInt(tc.raw), "EnvInt(%q)", tc.raw)
	}
}

func TestIsTruthy(t *testing.T) {
	truthy := []string{"1", "true", "yes", "on", "TRUE", " On "}
	for _, raw := range truthy {
		assert.Truef(t, IsTruthy(raw), "IsTruthy(%q)", raw)
	}
	falsy := []string{"", "0", "false", "no", "off", "banana"}
	for _, raw := range falsy {
		assert.Falsef(t, IsTruthy(raw), "IsTruthy(%q)", raw)
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
		{"empty takes the default", "", run.DefaultCommandTimeout, false},
		{"a duration is honoured", "3s", 3 * time.Second, false},
		{"zero is refused", "0s", run.DefaultCommandTimeout, true},
		{"negative is refused", "-5s", run.DefaultCommandTimeout, true},
		{"malformed is refused", "soon", run.DefaultCommandTimeout, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CommandTimeout(tc.raw)
			assert.Equalf(t, tc.want, got, "CommandTimeout(%q)", tc.raw)
			if tc.wantErr {
				assert.Errorf(t, err, "CommandTimeout(%q) must be reported", tc.raw)
			} else {
				assert.NoErrorf(t, err, "CommandTimeout(%q)", tc.raw)
			}
		})
	}
}

func TestInteractiveTimeout(t *testing.T) {
	got, err := InteractiveTimeout("")
	assert.NoError(t, err, `InteractiveTimeout("")`)
	assert.Equal(t, run.DefaultInteractiveTimeout, got, `InteractiveTimeout("")`)

	got, err = InteractiveTimeout("90s")
	assert.NoError(t, err, `InteractiveTimeout("90s")`)
	assert.Equal(t, 90*time.Second, got, `InteractiveTimeout("90s")`)

	got, err = InteractiveTimeout("0")
	assert.Error(t, err, `InteractiveTimeout("0") must be refused`)
	assert.Equal(t, run.DefaultInteractiveTimeout, got, `InteractiveTimeout("0") must fall back to the default`)
}
