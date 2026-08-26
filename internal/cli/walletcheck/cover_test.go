package walletcheck

import (
	"errors"
	"slices"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errNotFound is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errNotFound = errors.New("not found")

// probeWith builds a walletProbe answering exactly what a case describes, so
// each branch is reached from any platform rather than only from the one the
// test happens to run on.
func probeWith(goos string, found []string, present []string, bus string, listening []string) walletProbe {
	has := func(list []string, want string) bool {
		return slices.Contains(list, want)
	}
	return walletProbe{
		goos: goos,
		onPath: func(name string) (string, error) {
			if has(found, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errNotFound
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
	require.FailNowf(t, "the report is missing a requirement it must carry", "no %q in %+v", name, view.Requirements)
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
			view := walletView(t.Context(), tc.settings, tc.probe)
			assert.Equal(t, tc.wantBackend, view.Backend, "the wallet the report names")
			assert.Equal(t, tc.wantRoute, view.Route, "and how it would be reached")
			if tc.wantReq == "" {
				assert.Empty(t, view.Requirements, "a wallet with nothing to check needs nothing listed")
				return
			}
			req := requirement(t, view, tc.wantReq)
			assert.Equalf(t, tc.wantPresent, req.Present, "whether %s is there (detail %q)", req.Name, req.Detail)
			assert.Containsf(t, req.Detail, tc.wantDetail, "what the report says about %s", req.Name)
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
		look             SecretServiceLook
		hasScreen        bool
		wantService      string // a fragment of the secret service detail
		wantServiceThere bool   // whether the wallet counts as reachable
		wantPresent      bool   // whether the compartment is there
		wantFixable      bool   // whether this session could go and make one
		wantUndetermined bool
		wantCompartment  string // a fragment of the compartment detail
	}{
		{
			name:             "a wallet is answering and the compartment is there",
			look:             SecretServiceLook{Running: true, CollectionFound: true},
			wantService:      "answering",
			wantServiceThere: true,
			wantPresent:      true,
			wantCompartment:  "sshakku",
		},
		{
			name:             "no compartment, but a screen to make one on",
			look:             SecretServiceLook{Running: true},
			hasScreen:        true,
			wantService:      "answering",
			wantServiceThere: true,
			wantFixable:      true,
			wantCompartment:  "the first passphrase saved creates it",
		},
		{
			name:             "no compartment and no screen: nothing can ever be saved",
			look:             SecretServiceLook{Running: true},
			wantService:      "answering",
			wantServiceThere: true,
			wantCompartment:  "no screen to create it on",
		},
		{
			name:             "a wallet that did not answer is not a wallet that is empty",
			look:             SecretServiceLook{Running: true, AskFailed: true},
			wantService:      "answering",
			wantServiceThere: true,
			wantUndetermined: true,
			wantCompartment:  "did not answer",
		},
		{
			name: "a wallet that is not running yet is started when needed",
			look: SecretServiceLook{Activatable: true},
			// Nothing has asked for it yet, so nothing is missing: reported as
			// a missing piece it becomes a finding telling the user something
			// is wrong with a machine that is working.
			wantService:      "not running",
			wantServiceThere: true,
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
		{
			name:             "nothing on the bus, and nothing that would start",
			look:             SecretServiceLook{},
			wantService:      "nothing answers to org.freedesktop.secrets",
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
		{
			name:             "the bus itself could not be asked",
			look:             SecretServiceLook{LookFailed: true},
			wantService:      "could not be asked",
			wantUndetermined: true,
			wantCompartment:  "no wallet was answering",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := serviceRequirement(tc.look)
			assert.Contains(t, service.Detail, tc.wantService, "what the report says about the wallet itself")
			assert.Equalf(t, tc.wantServiceThere, service.Present,
				"whether the wallet counts as reachable (detail %q)", service.Detail)

			// Four separate claims about one requirement — there, fixable,
			// undetermined, and what it says — each worth naming on its own.
			compartment := CompartmentRequirement("sshakku", tc.look, tc.hasScreen)
			assert.Equalf(t, tc.wantPresent, compartment.Present, "compartment present (detail %q)", compartment.Detail)
			assert.Equalf(t, tc.wantFixable, compartment.Fixable, "compartment fixable (detail %q)", compartment.Detail)
			assert.Equalf(t, tc.wantUndetermined, compartment.Undetermined,
				"compartment undetermined (detail %q)", compartment.Detail)
			assert.Contains(t, compartment.Detail, tc.wantCompartment, "what the report says about the compartment")
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

	assert.Empty(t, diagnose.WalletFindings(view),
		"findings are what the user is told is wrong, and what nobody established is not wrong")
}

// realProbe reads the machine rather than a fixture, so all that can be
// checked here is that it is wired to something at all — an unwired field would
// make every check silently answer "missing".
func TestRealWalletProbeIsWired(t *testing.T) {
	probe := realProbe()
	// An unwired field would make every check silently answer "missing", which
	// reads exactly like a machine with nothing installed on it.
	require.NotNil(t, probe.onPath, "the probe must be able to look on PATH")
	require.NotNil(t, probe.exists, "and to look for a file")
	assert.NotEmpty(t, probe.goos, "the probe must name the platform it is on")
	assert.NotEmpty(t, probe.listening, "and know where KeePassXC would listen")

	_, err := probe.onPath("a-command-that-does-not-exist")
	assert.Error(t, err, "a command that is nowhere must be reported as nowhere")
	assert.False(t, probe.exists("/nonexistent/for/sure"), "and a path that is not there as not there")
}

// TestRealWalletProbeSeesEitherKindOfScreen covers what the answer decides: a
// session with no screen is told passphrases cannot be saved on it at all. Read
// from X11 alone, an ordinary Wayland desktop is that session — which is what
// the report would then say about a machine that works perfectly.
func TestRealWalletProbeSeesEitherKindOfScreen(t *testing.T) {
	t.Run("X11", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0")
		t.Setenv("WAYLAND_DISPLAY", "")
		assert.True(t, realProbe().hasScreen, "an X11 session has a screen")
	})

	t.Run("Wayland", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "wayland-0")
		assert.True(t, realProbe().hasScreen, "so does a Wayland session, with no X11 to show for it")
	})

	t.Run("neither", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		assert.False(t, realProbe().hasScreen, "and a session reached over ssh has neither")
	})
}
