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
		{"empty means SSHakku chooses", ptr(""), KeePassXCRouteAuto, false},
		{"auto", ptr(KeePassXCRouteAuto), KeePassXCRouteAuto, false},
		{"secret-service", ptr(KeePassXCRouteSecretService), KeePassXCRouteSecretService, false},
		{"native", ptr(KeePassXCRouteNative), KeePassXCRouteNative, false},
		{"cli", ptr(KeePassXCRouteCLI), KeePassXCRouteCLI, false},
		// A typo must not silently pin something: it falls back to choosing,
		// and says so.
		{"an unknown route is reported", ptr("nativ"), KeePassXCRouteAuto, true},
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
	s, errs := Resolve(File{KeePassXCRoute: ptr("browser")}, lookupFrom(nil))
	assert.NotEmpty(t, errs, "an unrecognised route must be reported")
	assert.Equal(t, KeePassXCRouteAuto, s.KeePassXCRoute, "route must fall back")
}

func TestMergeOverridesTheKeePassXCSettings(t *testing.T) {
	base := File{
		KeePassXCRoute:    ptr(KeePassXCRouteNative),
		KeePassXCDatabase: ptr("/base.kdbx"),
		KeePassXCKeyFile:  ptr("/base.key"),
	}
	other := File{
		KeePassXCRoute:    ptr(KeePassXCRouteCLI),
		KeePassXCDatabase: ptr("/other.kdbx"),
		KeePassXCKeyFile:  ptr("/other.key"),
	}
	got := base.Merge(other)
	assert.Equal(t, ptr(KeePassXCRouteCLI), got.KeePassXCRoute, "route must be other's")
	assert.Equal(t, ptr("/other.kdbx"), got.KeePassXCDatabase, "database must be other's")
	assert.Equal(t, ptr("/other.key"), got.KeePassXCKeyFile, "key file must be other's")
}

func TestMergeKeepsTheBaseKeePassXCSettingsWhenOtherIsSilent(t *testing.T) {
	base := File{
		KeePassXCRoute:    ptr(KeePassXCRouteNative),
		KeePassXCDatabase: ptr("/base.kdbx"),
		KeePassXCKeyFile:  ptr("/base.key"),
	}
	got := base.Merge(File{})
	assert.Equal(t, ptr(KeePassXCRouteNative), got.KeePassXCRoute, "route must be the base's")
	assert.Equal(t, ptr("/base.kdbx"), got.KeePassXCDatabase, "database must be the base's")
	assert.Equal(t, ptr("/base.key"), got.KeePassXCKeyFile, "key file must be the base's")
}
