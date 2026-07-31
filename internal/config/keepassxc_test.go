package config

import "testing"

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
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("route = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveAcceptsKeePassXCAsABackend(t *testing.T) {
	s, errs := Resolve(File{SecretBackend: ptr(SecretBackendKeePassXC)}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.SecretBackend != SecretBackendKeePassXC {
		t.Errorf("SecretBackend = %q, want keepassxc — the wallet is named, not the mechanism", s.SecretBackend)
	}
}

func TestResolveCarriesTheKeePassXCSettings(t *testing.T) {
	s, errs := Resolve(File{
		SecretBackend:     ptr(SecretBackendKeePassXC),
		KeePassXCRoute:    ptr(KeePassXCRouteCLI),
		KeePassXCDatabase: ptr("/home/someone/secrets.kdbx"),
		KeePassXCKeyFile:  ptr("/home/someone/secrets.key"),
	}, lookupFrom(nil))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if s.KeePassXCRoute != KeePassXCRouteCLI {
		t.Errorf("route = %q, want cli", s.KeePassXCRoute)
	}
	if s.KeePassXCDatabase != "/home/someone/secrets.kdbx" || s.KeePassXCKeyFile != "/home/someone/secrets.key" {
		t.Errorf("database/key file = %q/%q, want what the file said", s.KeePassXCDatabase, s.KeePassXCKeyFile)
	}
}

func TestResolveReportsAnInvalidKeePassXCRoute(t *testing.T) {
	s, errs := Resolve(File{KeePassXCRoute: ptr("browser")}, lookupFrom(nil))
	if len(errs) == 0 {
		t.Fatal("an unrecognised route must be reported")
	}
	if s.KeePassXCRoute != KeePassXCRouteAuto {
		t.Errorf("route = %q, want the fallback", s.KeePassXCRoute)
	}
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
	if got.KeePassXCRoute == nil || *got.KeePassXCRoute != KeePassXCRouteCLI {
		t.Errorf("route = %v, want other's", got.KeePassXCRoute)
	}
	if got.KeePassXCDatabase == nil || *got.KeePassXCDatabase != "/other.kdbx" {
		t.Errorf("database = %v, want other's", got.KeePassXCDatabase)
	}
	if got.KeePassXCKeyFile == nil || *got.KeePassXCKeyFile != "/other.key" {
		t.Errorf("key file = %v, want other's", got.KeePassXCKeyFile)
	}
}

func TestMergeKeepsTheBaseKeePassXCSettingsWhenOtherIsSilent(t *testing.T) {
	base := File{
		KeePassXCRoute:    ptr(KeePassXCRouteNative),
		KeePassXCDatabase: ptr("/base.kdbx"),
		KeePassXCKeyFile:  ptr("/base.key"),
	}
	got := base.Merge(File{})
	if got.KeePassXCRoute == nil || *got.KeePassXCRoute != KeePassXCRouteNative {
		t.Errorf("route = %v, want the base's", got.KeePassXCRoute)
	}
	if got.KeePassXCDatabase == nil || *got.KeePassXCDatabase != "/base.kdbx" {
		t.Errorf("database = %v, want the base's", got.KeePassXCDatabase)
	}
	if got.KeePassXCKeyFile == nil || *got.KeePassXCKeyFile != "/base.key" {
		t.Errorf("key file = %v, want the base's", got.KeePassXCKeyFile)
	}
}
