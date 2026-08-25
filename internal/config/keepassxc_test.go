package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveKeePassXCRoute(t *testing.T) {
	tests := []struct {
		name    string
		file    *string
		want    string
		wantErr bool
	}{
		{"absent means SSHakku chooses", nil, KeePassXCRouteAuto, false},
		{"empty means SSHakku chooses", new(""), KeePassXCRouteAuto, false},
		{"auto", new(KeePassXCRouteAuto), KeePassXCRouteAuto, false},
		{"secret-service", new(KeePassXCRouteSecretService), KeePassXCRouteSecretService, false},
		{"native", new(KeePassXCRouteNative), KeePassXCRouteNative, false},
		{"cli", new(KeePassXCRouteCLI), KeePassXCRouteCLI, false},
		// A typo must not silently pin something: it falls back to choosing,
		// and says so.
		{"an unknown route is reported", new("nativ"), KeePassXCRouteAuto, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveKeePassXCRoute(tc.file)
			if tc.wantErr {
				assert.Error(t, err, "an unrecognised route must be reported")
			} else {
				assert.NoError(t, err, "a recognised route must not be reported")
			}
			assert.Equal(t, tc.want, got, "route")
		})
	}
}

// The two tests that name KeePassXC as the wallet — rather than only pinning
// the route to it — need this system to have a wallet that can be named at
// all, so they are in wallet_unix_test.go.

func TestResolveReportsAnInvalidKeePassXCRoute(t *testing.T) {
	s, errs := Resolve(File{KeePassXCRoute: new("browser")}, lookupFrom(nil))
	assert.NotEmpty(t, errs, "an unrecognised route must be reported")
	assert.Equal(t, KeePassXCRouteAuto, s.KeePassXCRoute, "route must fall back")
}

func TestMergeOverridesTheKeePassXCSettings(t *testing.T) {
	base := File{
		KeePassXCRoute:    new(KeePassXCRouteNative),
		KeePassXCDatabase: new("/base.kdbx"),
		KeePassXCKeyFile:  new("/base.key"),
	}
	other := File{
		KeePassXCRoute:    new(KeePassXCRouteCLI),
		KeePassXCDatabase: new("/other.kdbx"),
		KeePassXCKeyFile:  new("/other.key"),
	}
	got := base.Merge(other)
	assert.Equal(t, new(KeePassXCRouteCLI), got.KeePassXCRoute, "route must be other's")
	assert.Equal(t, new("/other.kdbx"), got.KeePassXCDatabase, "database must be other's")
	assert.Equal(t, new("/other.key"), got.KeePassXCKeyFile, "key file must be other's")
}

func TestMergeKeepsTheBaseKeePassXCSettingsWhenOtherIsSilent(t *testing.T) {
	base := File{
		KeePassXCRoute:    new(KeePassXCRouteNative),
		KeePassXCDatabase: new("/base.kdbx"),
		KeePassXCKeyFile:  new("/base.key"),
	}
	got := base.Merge(File{})
	assert.Equal(t, new(KeePassXCRouteNative), got.KeePassXCRoute, "route must be the base's")
	assert.Equal(t, new("/base.kdbx"), got.KeePassXCDatabase, "database must be the base's")
	assert.Equal(t, new("/base.key"), got.KeePassXCKeyFile, "key file must be the base's")
}
