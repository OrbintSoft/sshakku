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

// withLook fixes what looking at the session bus found, and whether the session
// has a screen, so every combination of the two can be described from a machine
// that has neither.
func (p walletProbe) withLook(look secretServiceLook, hasScreen bool) walletProbe {
	p.look = func(string, string) secretServiceLook { return look }
	p.hasScreen = hasScreen
	return p
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

// TestSecretServiceRequirementsFromALook verifies F25 and F41 for every answer
// a look can come back with. It runs on any platform, including one with no
// session bus at all: what the report makes of a finding is a decision about
// words, and a decision about words is checkable anywhere.
func TestSecretServiceRequirementsFromALook(t *testing.T) {
	tests := []struct {
		name             string
		look             secretServiceLook
		hasScreen        bool
		wantService      string // a fragment of the secret service detail
		wantPresent      bool   // whether the compartment is there
		wantFixable      bool   // whether this session could go and make one
		wantUndetermined bool
		wantCompartment  string // a fragment of the compartment detail
	}{
		{
			name:            "a wallet is answering and the compartment is there",
			look:            secretServiceLook{running: true, collectionFound: true},
			wantService:     "answering",
			wantPresent:     true,
			wantCompartment: "sshakku",
		},
		{
			name:            "no compartment, but a screen to make one on",
			look:            secretServiceLook{running: true},
			hasScreen:       true,
			wantService:     "answering",
			wantFixable:     true,
			wantCompartment: "the first passphrase saved creates it",
		},
		{
			name:            "no compartment and no screen: nothing can ever be saved",
			look:            secretServiceLook{running: true},
			wantService:     "answering",
			wantCompartment: "no screen to create it on",
		},
		{
			name:             "a wallet that did not answer is not a wallet that is empty",
			look:             secretServiceLook{running: true, askFailed: true},
			wantService:      "answering",
			wantUndetermined: true,
			wantCompartment:  "did not answer",
		},
		{
			name:             "a wallet that is not running yet is started when needed",
			look:             secretServiceLook{activatable: true},
			wantService:      "not running",
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
		{
			name:             "nothing on the bus, and nothing that would start",
			look:             secretServiceLook{},
			wantService:      "nothing answers to org.freedesktop.secrets",
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
		{
			name:             "the bus itself could not be asked",
			look:             secretServiceLook{lookFailed: true},
			wantService:      "could not be asked",
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := serviceRequirement(tc.look)
			if !strings.Contains(service.Detail, tc.wantService) {
				t.Errorf("secret service detail = %q, want it to contain %q", service.Detail, tc.wantService)
			}

			compartment := compartmentRequirement("sshakku", tc.look, tc.hasScreen)
			if compartment.Present != tc.wantPresent {
				t.Errorf("compartment present = %v, want %v (detail %q)",
					compartment.Present, tc.wantPresent, compartment.Detail)
			}
			if compartment.Fixable != tc.wantFixable {
				t.Errorf("compartment fixable = %v, want %v (detail %q)",
					compartment.Fixable, tc.wantFixable, compartment.Detail)
			}
			if compartment.Undetermined != tc.wantUndetermined {
				t.Errorf("compartment undetermined = %v, want %v (detail %q)",
					compartment.Undetermined, tc.wantUndetermined, compartment.Detail)
			}
			if !strings.Contains(compartment.Detail, tc.wantCompartment) {
				t.Errorf("compartment detail = %q, want it to contain %q", compartment.Detail, tc.wantCompartment)
			}
		})
	}
}

// TestAnUndeterminedRequirementIsNotAFinding keeps the two apart where it
// matters: findings are what a user is told is wrong, and something nobody
// established is not wrong.
func TestAnUndeterminedRequirementIsNotAFinding(t *testing.T) {
	// The wallet is named as a plain string: which one it is decides nothing
	// here, and the name of the one this case describes is a name only the
	// platform that has it defines.
	view := diagnose.WalletView{
		Backend: "secret-service",
		Requirements: []diagnose.Requirement{
			{Name: "session bus", Detail: "unix:path=/run/bus", Present: true},
			{Name: "compartment", Detail: "no wallet was answering to ask", Undetermined: true},
		},
	}

	if findings := diagnose.WalletFindings(view); len(findings) != 0 {
		t.Errorf("findings = %q, want none for a requirement nobody could settle", findings)
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
