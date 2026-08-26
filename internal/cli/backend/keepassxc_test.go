package backend

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc"
)

// keepassxcSettings names the wallet and pins the route.
func keepassxcSettings(route string) config.Settings {
	return config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: route,
	}
}

// tempRuntimeEnv points HOME and the XDG dirs at fresh temp dirs so the runtime
// layout is built and created entirely off the real state, runtime, and config
// dirs.
func tempRuntimeEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	return home
}

// TestKeePassXCPinnedNativeRouteIsUsedWhereItCanBeReached states the rule the
// routes exist under: a route is available wherever it can work, not only on
// the OS where SSHakku would have picked it. A Linux user who does not want the
// Secret Service can say so and be taken at their word.
//
// Windows is the one place it cannot be reached, and it is asserted here rather
// than in a test of its own so the rule and its exception are read together.
// The platform is named rather than taken from the machine: read from
// runtime.GOOS this would assert one row and leave every other unexercised,
// which is how the exception went unnoticed — the assertion passed on Windows
// by agreeing with a build that would then have dialled a path that is not
// there.
func TestKeePassXCPinnedNativeRouteIsUsedWhereItCanBeReached(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "freebsd"} {
		t.Run(goos, func(t *testing.T) {
			backend, closeFn := newKeePassXCBackend(
				t.Context(), goos, "alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteNative))
			defer closeFn()

			assert.IsTypef(t, keepassxc.Native{}, backend,
				"a route pinned by the user must be taken at their word on %s", goos)
		})
	}

	t.Run("windows", func(t *testing.T) {
		backend, closeFn := newKeePassXCBackend(
			t.Context(), "windows", "alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteNative))
		defer closeFn()

		unavailable, ok := backend.(wallet.Unavailable)
		require.Truef(t, ok, "a route this build cannot reach must say so, got %T", backend)
		assert.ErrorContains(t, unavailable.Reason, "native",
			"the reason must name the route the user asked for")
	})
}

// TestKeePassXCPinnedSecretServiceRouteOffLinuxSaysWhichRouteFailed is the
// other half of the same rule: a pinned route that cannot work is reported
// under its own name rather than silently swapped for another, which would
// answer a question the user did not ask.
func TestKeePassXCPinnedSecretServiceRouteIsHonoured(t *testing.T) {
	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteSecretService))
	defer closeFn()

	if runtime.GOOS != "linux" {
		unavailable, ok := backend.(wallet.Unavailable)
		require.Truef(t, ok, "a route that cannot work here must say so, got %T", backend)
		assert.ErrorContains(t, unavailable.Reason, "secret-service",
			"the reason must name the route the user asked for, not another one")
		_, _, err := backend.Lookup(t.Context(), "id_ed25519")
		assert.Error(t, err, "an unavailable route must fail rather than report an empty wallet")
		return
	}
	_, isNative := backend.(keepassxc.Native)
	assert.False(t, isNative, "pinning secret-service must not quietly hand back the native route")
}

// TestKeePassXCSecretServiceRouteHandsOffToTheDefaultBackend states what that
// route is: KeePassXC implements the Secret Service API itself, so reaching it
// that way is reaching any other wallet behind that API, and the KeePassXC
// backend must not appear.
//
// The platform is named rather than taken from the machine, because that route
// exists on Linux alone: read from runtime.GOOS, this branch would be
// unreachable — and so untested — on every other platform the product is built
// for, which is exactly what it was.
func TestKeePassXCSecretServiceRouteHandsOffToTheDefaultBackend(t *testing.T) {
	settings := keepassxcSettings(config.KeePassXCRouteSecretService)

	got, closeGot := newKeePassXCBackend(t.Context(), "linux", "alice", fakeLogger{}, settings)
	defer closeGot()
	want, closeWant := newDefaultSecretBackend(t.Context(), "alice", fakeLogger{}, settings)
	defer closeWant()

	assert.Equal(t, fmt.Sprintf("%T", want), fmt.Sprintf("%T", got),
		"reaching KeePassXC through the Secret Service is reaching any other wallet behind that API")
	_, isKeePassXC := got.(keepassxc.Native)
	assert.False(t, isKeePassXC, "it must not open KeePassXC's own protocol instead of the shared API")
}

// TestKeePassXCAutoRouteFollowsThePlatform covers the one value that chooses.
// The choice is taken as a function of the platform so both answers can be
// checked from either one, rather than each OS testing only its own half.
func TestKeePassXCAutoRouteFollowsThePlatform(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{"linux", config.KeePassXCRouteSecretService},
		{"darwin", config.KeePassXCRouteNative},
		{"freebsd", config.KeePassXCRouteNative},
		{"windows", config.KeePassXCRouteCLI},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			assert.Equalf(t, tc.want, KeePassXCRouteFor(tc.goos), "the route chosen on %s", tc.goos)
		})
	}
}

// TestKeePassXCRouteAvailability states which routes can work where. Only the
// Secret Service is platform-bound, because it is an API macOS does not have;
// the rest are available wherever they can run.
func TestKeePassXCRouteAvailability(t *testing.T) {
	assert.NoError(t, keepassxcRouteUnavailable(config.KeePassXCRouteSecretService, "linux", ""),
		"the Secret Service exists on Linux")

	err := keepassxcRouteUnavailable(config.KeePassXCRouteSecretService, "darwin", "")
	require.Error(t, err, "macOS has no Secret Service, so pinning it must be reported")
	assert.ErrorContains(t, err, "secret-service", "the reason must name the route")
	assert.ErrorContains(t, err, "darwin", "and the platform it cannot work on")

	assert.NoError(t, keepassxcRouteUnavailable(config.KeePassXCRouteNative, "darwin", ""),
		"the native route runs anywhere it can be found")
	assert.NoError(t, keepassxcRouteUnavailable(config.KeePassXCRouteNative, "linux", ""),
		"including where another route would have been chosen for you")
}

// TestKeePassXCNativeRouteSaysWhatItIsThatWindowsHasNot keeps two refusals
// apart that would otherwise read alike. macOS has no Secret Service and never
// will: nothing anybody installs makes that route appear. Windows has the thing
// the native route wants — KeePassXC serves its local protocol there as well —
// and what is missing is on this side: it is served over a named pipe, and
// SSHakku knows how to find a socket. A message blaming the platform for that
// would send somebody looking for a setting in KeePassXC.
func TestKeePassXCNativeRouteSaysWhatItIsThatWindowsHasNot(t *testing.T) {
	err := keepassxcRouteUnavailable(config.KeePassXCRouteNative, "windows", "")

	require.Error(t, err, "a route this build cannot reach must be reported rather than dialled")
	assert.ErrorContains(t, err, "native", "the reason must name the route")
	assert.ErrorContains(t, err, "windows", "and the platform it was asked for on")
	assert.ErrorContains(t, err, "named pipe", "and what KeePassXC is actually serving there")
	assert.ErrorContains(t, err, "cli", "and the route that does work here, since one does")
}

// TestKeePassXCCLIRouteNeedsADatabase states what the CLI route is bound to.
// It is not a platform: a file on disk does not announce itself the way a
// running KeePassXC knows what it has open, so the user has to say which one.
func TestKeePassXCCLIRouteNeedsADatabase(t *testing.T) {
	err := keepassxcRouteUnavailable(config.KeePassXCRouteCLI, "linux", "")
	require.Error(t, err, "the cli route with no database named must be reported")
	assert.ErrorContains(t, err, "keepassxc_database", "the reason must name the setting the user has to write")

	for _, goos := range []string{"linux", "darwin", "freebsd"} {
		assert.NoErrorf(t, keepassxcRouteUnavailable(config.KeePassXCRouteCLI, goos, "/secrets.kdbx"),
			"a named database is all the cli route needs, on %s as anywhere", goos)
	}
}

// TestKeePassXCEmptyRouteIsTheAutomaticOne guards the default a user gets by
// naming only the wallet: not merely that something opens, but that it is the
// route this platform would have chosen. Both spellings of "choose for me" —
// the empty string and the word — have to arrive at the same place, and the
// platform is passed in so both answers are checkable from either machine.
func TestKeePassXCEmptyRouteIsTheAutomaticOne(t *testing.T) {
	for _, route := range []string{"", config.KeePassXCRouteAuto} {
		t.Run("route "+route, func(t *testing.T) {
			backend, closeFn := newKeePassXCBackend(t.Context(), "darwin", "alice", fakeLogger{}, keepassxcSettings(route))
			defer closeFn()

			assert.IsType(t, keepassxc.Native{}, backend,
				"a platform with no Secret Service must land on the native route, not fall through to it")
		})
	}
}

func TestKeePassXCPinnedCLIRouteIsBuiltWhenItHasADatabase(t *testing.T) {
	settings := keepassxcSettings(config.KeePassXCRouteCLI)
	settings.KeePassXCDatabase = "/somewhere/secrets.kdbx"
	settings.KeePassXCKeyFile = "/somewhere/secrets.key"
	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, settings)
	defer closeFn()

	cli, ok := backend.(*keepassxc.CLI)
	require.Truef(t, ok, "the route pinned must be the one built, got %T", backend)
	assert.Equal(t, "/somewhere/secrets.kdbx", cli.Database, "the database the configuration named")
	assert.Equal(t, "/somewhere/secrets.key", cli.KeyFile, "the key file the configuration named")
	assert.NotNil(t, cli.Prompter, "this route has to ask for the database password, so it needs something to ask with")
}

func TestKeePassXCPinnedCLIRouteWithNoDatabaseReportsItself(t *testing.T) {
	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteCLI))
	defer closeFn()

	unavailable, ok := backend.(wallet.Unavailable)
	require.Truef(t, ok, "a route missing what it needs must say so, got %T", backend)
	assert.ErrorContains(t, unavailable.Reason, "keepassxc_database", "the reason must name what is missing")
}

// TestKeePassXCAssociationPathIsUnderTheStateDirectory keeps the approval out
// of the config directory, which a user may well keep in version control.
func TestKeePassXCAssociationPathIsUnderTheStateDirectory(t *testing.T) {
	tempRuntimeEnv(t)
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)

	path := keepassxcAssociationPath()
	require.NotEmpty(t, path, "the association must have somewhere to live")
	assert.True(t, strings.HasSuffix(path, "keepassxc-association.json"),
		"the file must be named for what it holds")
	assert.True(t, strings.HasPrefix(path, layout.StateDir),
		"an approval this machine gave belongs with the rest of this machine's state")
	assert.False(t, strings.HasPrefix(path, layout.ConfigDir),
		"and not in a directory a user may well keep in version control")
}

// TestKeePassXCNativeBackendCarriesItsAssociationStore proves the backend is
// wired to persist the approval; without it the user approves a dialog on
// every single run.
func TestKeePassXCNativeBackendCarriesItsAssociationStore(t *testing.T) {
	backend := newKeePassXCNativeRoute(keepassxcSettings(config.KeePassXCRouteNative))
	native, ok := backend.(keepassxc.Native)
	require.Truef(t, ok, "the native route must build the native backend, got %T", backend)
	require.NotNil(t, native.Associations,
		"without somewhere to remember its approval the user approves a dialog on every run")
	store, ok := native.Associations.(keepassxc.FileAssociationStore)
	require.Truef(t, ok, "the approval must outlive the process, got %T", native.Associations)
	assert.NotEmpty(t, store.Path, "and the store must know where to write it")
}
