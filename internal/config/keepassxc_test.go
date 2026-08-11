package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestResolveAcceptsKeePassXCAsABackend(t *testing.T) {
	s, errs := Resolve(File{SecretBackend: ptr(SecretBackendKeePassXC)}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, SecretBackendKeePassXC, s.SecretBackend, "the wallet is named, not the mechanism")
}

func TestResolveCarriesTheKeePassXCSettings(t *testing.T) {
	s, errs := Resolve(File{
		SecretBackend:     ptr(SecretBackendKeePassXC),
		KeePassXCRoute:    ptr(KeePassXCRouteCLI),
		KeePassXCDatabase: ptr("/home/someone/secrets.kdbx"),
		KeePassXCKeyFile:  ptr("/home/someone/secrets.key"),
	}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, KeePassXCRouteCLI, s.KeePassXCRoute, "route")
	assert.Equal(t, "/home/someone/secrets.kdbx", s.KeePassXCDatabase, "database")
	assert.Equal(t, "/home/someone/secrets.key", s.KeePassXCKeyFile, "key file")
}

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
