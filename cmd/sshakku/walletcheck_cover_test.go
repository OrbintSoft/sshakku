package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// probeWith builds a walletProbe answering exactly what a case describes, so
// each branch is reached from any platform rather than only from the one the
// test happens to run on.
func probeWith(goos string, found []string, present []string, bus string, listening []string) walletProbe {
	has := func(list []string, want string) bool {
		for _, item := range list {
			if item == want {
				return true
			}
		}
		return false
	}
	return walletProbe{
		goos: goos,
		onPath: func(name string) (string, error) {
			if has(found, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		exists:     func(path string) bool { return has(present, path) },
		busAddress: bus,
		listening:  listening,
	}
}

// requirement returns the named requirement, so a case can assert on the one
// it is about without depending on the order of the rest.
func requirement(t *testing.T, view diagnose.WalletView, name string) diagnose.Requirement {
	t.Helper()
	for _, req := range view.Requirements {
		if req.Name == name {
			return req
		}
	}
	t.Fatalf("no requirement named %q in %+v", name, view.Requirements)
	return diagnose.Requirement{}
}

func TestDefaultSecretBackendName(t *testing.T) {
	if got := defaultSecretBackendName("linux"); got != config.SecretBackendSecretService {
		t.Errorf("default on linux = %q, want the secret service", got)
	}
	if got := defaultSecretBackendName("darwin"); got != config.SecretBackendKeychain {
		t.Errorf("default off linux = %q, want the keychain", got)
	}
}

func TestWalletViewPerBackend(t *testing.T) {
	tests := []struct {
		name        string
		settings    config.Settings
		probe       walletProbe
		wantBackend string
		wantRoute   string
		wantReq     string // the requirement to inspect, "" for none
		wantPresent bool
		wantDetail  string // a fragment the detail must contain
	}{
		{
			name:        "no backend named falls back to the platform default",
			probe:       probeWith("linux", nil, nil, "unix:path=/run/bus", nil),
			wantBackend: config.SecretBackendSecretService,
			wantReq:     "session bus",
			wantPresent: true,
			wantDetail:  "unix:path=/run/bus",
		},
		{
			name:        "a session bus that is not there",
			settings:    config.Settings{SecretBackend: config.SecretBackendSecretService},
			probe:       probeWith("linux", nil, nil, "", nil),
			wantBackend: config.SecretBackendSecretService,
			wantReq:     "session bus",
			wantDetail:  "DBUS_SESSION_BUS_ADDRESS is unset",
		},
		{
			name:        "1password needs its own tool",
			settings:    config.Settings{SecretBackend: config.SecretBackendOnePassword},
			probe:       probeWith("linux", []string{"op"}, nil, "", nil),
			wantBackend: config.SecretBackendOnePassword,
			wantReq:     "op",
			wantPresent: true,
			wantDetail:  "/usr/bin/op",
		},
		{
			name:        "bitwarden without its tool",
			settings:    config.Settings{SecretBackend: config.SecretBackendBitwarden},
			probe:       probeWith("linux", nil, nil, "", nil),
			wantBackend: config.SecretBackendBitwarden,
			wantReq:     "bw",
			wantDetail:  "not on PATH",
		},
		{
			name:        "the keychain needs nothing else",
			settings:    config.Settings{SecretBackend: config.SecretBackendKeychain},
			probe:       probeWith("darwin", nil, nil, "", nil),
			wantBackend: config.SecretBackendKeychain,
		},
		{
			name: "keepassxc, cli route, everything there",
			settings: config.Settings{
				SecretBackend:     config.SecretBackendKeePassXC,
				KeePassXCRoute:    config.KeePassXCRouteCLI,
				KeePassXCDatabase: "/vault.kdbx",
			},
			probe:       probeWith("linux", []string{"keepassxc-cli"}, []string{"/vault.kdbx"}, "", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteCLI,
			wantReq:     "database",
			wantPresent: true,
			wantDetail:  "/vault.kdbx",
		},
		{
			name: "keepassxc, cli route, database named but absent",
			settings: config.Settings{
				SecretBackend:     config.SecretBackendKeePassXC,
				KeePassXCRoute:    config.KeePassXCRouteCLI,
				KeePassXCDatabase: "/gone.kdbx",
			},
			probe:       probeWith("linux", []string{"keepassxc-cli"}, nil, "", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteCLI,
			wantReq:     "database",
			wantDetail:  "/gone.kdbx does not exist",
		},
		{
			name: "keepassxc, native route, KeePassXC listening",
			settings: config.Settings{
				SecretBackend:  config.SecretBackendKeePassXC,
				KeePassXCRoute: config.KeePassXCRouteNative,
			},
			probe:       probeWith("linux", nil, []string{"/run/kpxc.sock"}, "", []string{"/nowhere", "/run/kpxc.sock"}),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteNative,
			wantReq:     "KeePassXC",
			wantPresent: true,
			wantDetail:  "/run/kpxc.sock",
		},
		{
			name: "keepassxc, native route, nothing listening",
			settings: config.Settings{
				SecretBackend:  config.SecretBackendKeePassXC,
				KeePassXCRoute: config.KeePassXCRouteNative,
			},
			probe:       probeWith("linux", nil, nil, "", []string{"/nowhere"}),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteNative,
			wantReq:     "KeePassXC",
			wantDetail:  "nothing is listening",
		},
		{
			name:        "keepassxc with no route named takes the platform's own",
			settings:    config.Settings{SecretBackend: config.SecretBackendKeePassXC},
			probe:       probeWith("darwin", nil, nil, "", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteNative,
			wantReq:     "KeePassXC",
			wantDetail:  "nothing is listening",
		},
		{
			name: "keepassxc over a secret service that the platform does not have",
			settings: config.Settings{
				SecretBackend:  config.SecretBackendKeePassXC,
				KeePassXCRoute: config.KeePassXCRouteSecretService,
			},
			probe:       probeWith("darwin", nil, nil, "", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteSecretService,
			wantReq:     "secret service",
			wantDetail:  "darwin provides no freedesktop Secret Service",
		},
		{
			name: "keepassxc over the secret service on linux",
			settings: config.Settings{
				SecretBackend:  config.SecretBackendKeePassXC,
				KeePassXCRoute: config.KeePassXCRouteAuto,
			},
			probe:       probeWith("linux", nil, nil, "unix:path=/run/bus", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteSecretService,
			wantReq:     "session bus",
			wantPresent: true,
			wantDetail:  "unix:path=/run/bus",
		},
		{
			name: "keepassxc, cli route, no database named at all",
			settings: config.Settings{
				SecretBackend:  config.SecretBackendKeePassXC,
				KeePassXCRoute: config.KeePassXCRouteCLI,
			},
			probe:       probeWith("linux", []string{"keepassxc-cli"}, nil, "", nil),
			wantBackend: config.SecretBackendKeePassXC,
			wantRoute:   config.KeePassXCRouteCLI,
			wantReq:     "database",
			wantDetail:  "keepassxc_database has to name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			view := walletView(tc.settings, tc.probe)
			if view.Backend != tc.wantBackend {
				t.Errorf("backend = %q, want %q", view.Backend, tc.wantBackend)
			}
			if view.Route != tc.wantRoute {
				t.Errorf("route = %q, want %q", view.Route, tc.wantRoute)
			}
			if tc.wantReq == "" {
				if len(view.Requirements) != 0 {
					t.Errorf("requirements = %+v, want none", view.Requirements)
				}
				return
			}
			req := requirement(t, view, tc.wantReq)
			if req.Present != tc.wantPresent {
				t.Errorf("%s present = %v, want %v (detail %q)", req.Name, req.Present, tc.wantPresent, req.Detail)
			}
			if !strings.Contains(req.Detail, tc.wantDetail) {
				t.Errorf("%s detail = %q, want it to contain %q", req.Name, req.Detail, tc.wantDetail)
			}
		})
	}
}

// realWalletProbe reads the machine rather than a fixture, so all that can be
// checked here is that it is wired to something at all — an unwired field would
// make every check silently answer "missing".
func TestRealWalletProbeIsWired(t *testing.T) {
	probe := realWalletProbe()
	if probe.onPath == nil || probe.exists == nil {
		t.Fatal("realWalletProbe must supply both lookups")
	}
	if probe.goos == "" {
		t.Error("realWalletProbe must name the platform")
	}
	if len(probe.listening) == 0 {
		t.Error("realWalletProbe must know where KeePassXC would listen")
	}
	if _, err := probe.onPath("a-command-that-does-not-exist"); err == nil {
		t.Error("onPath must report a command that is nowhere")
	}
	if probe.exists("/nonexistent/for/sure") {
		t.Error("exists must report a path that is not there")
	}
}
