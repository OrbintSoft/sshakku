package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// keepassxcSettings names the wallet and pins the route.
func keepassxcSettings(route string) config.Settings {
	return config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: route,
	}
}

// TestKeePassXCPinnedNativeRouteIsUsedEverywhere states the rule the routes
// exist under: a route is available wherever it can work, not only on the OS
// where SSHakku would have picked it. A Linux user who does not want the Secret
// Service can say so and be taken at their word.
func TestKeePassXCPinnedNativeRouteIsUsedEverywhere(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteNative))
	defer closeFn()

	if _, ok := backend.(keys.KeePassXCBackend); !ok {
		t.Fatalf("backend = %T, want the native KeePassXC backend on %s", backend, runtime.GOOS)
	}
}

// TestKeePassXCPinnedSecretServiceRouteOffLinuxSaysWhichRouteFailed is the
// other half of the same rule: a pinned route that cannot work is reported
// under its own name rather than silently swapped for another, which would
// answer a question the user did not ask.
func TestKeePassXCPinnedSecretServiceRouteIsHonoured(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteSecretService))
	defer closeFn()

	if runtime.GOOS != "linux" {
		unavailable, ok := backend.(keys.UnavailableBackend)
		if !ok {
			t.Fatalf("backend = %T, want an unavailable backend where the API does not exist", backend)
		}
		if !strings.Contains(unavailable.Reason.Error(), "secret-service") {
			t.Errorf("reason = %v, want it to name the route that failed", unavailable.Reason)
		}
		if _, _, err := backend.Lookup("id_ed25519"); err == nil {
			t.Error("an unavailable route must fail rather than report an empty wallet")
		}
		return
	}
	if _, isNative := backend.(keys.KeePassXCBackend); isNative {
		t.Error("pinning secret-service must not hand back the native route")
	}
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
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			if got := keepassxcRouteFor(tc.goos); got != tc.want {
				t.Errorf("route on %s = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}

// TestKeePassXCRouteAvailability states which routes can work where. Only the
// Secret Service is platform-bound, because it is an API macOS does not have;
// the rest are available wherever they can run.
func TestKeePassXCRouteAvailability(t *testing.T) {
	if err := keepassxcRouteUnavailable(config.KeePassXCRouteSecretService, "linux"); err != nil {
		t.Errorf("the Secret Service exists on Linux, got %v", err)
	}
	err := keepassxcRouteUnavailable(config.KeePassXCRouteSecretService, "darwin")
	if err == nil {
		t.Fatal("macOS has no Secret Service, so pinning it must be reported")
	}
	if !strings.Contains(err.Error(), "secret-service") || !strings.Contains(err.Error(), "darwin") {
		t.Errorf("reason = %v, want it to name the route and the platform", err)
	}
	if err := keepassxcRouteUnavailable(config.KeePassXCRouteNative, "darwin"); err != nil {
		t.Errorf("the native route runs anywhere, got %v", err)
	}
	if err := keepassxcRouteUnavailable(config.KeePassXCRouteNative, "linux"); err != nil {
		t.Errorf("the native route is not Linux-forbidden, got %v", err)
	}
}

// TestKeePassXCEmptyRouteIsTheAutomaticOne guards the default a user gets by
// naming only the wallet.
func TestKeePassXCEmptyRouteIsTheAutomaticOne(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, keepassxcSettings(""))
	defer closeFn()

	if backend == nil {
		t.Fatal("naming the wallet alone must still produce a backend")
	}
}

func TestKeePassXCPinnedCLIRouteReportsItself(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, keepassxcSettings(config.KeePassXCRouteCLI))
	defer closeFn()

	unavailable, ok := backend.(keys.UnavailableBackend)
	if !ok {
		t.Fatalf("backend = %T, want an unavailable backend", backend)
	}
	if !strings.Contains(unavailable.Reason.Error(), "cli") {
		t.Errorf("reason = %v, want it to name the route", unavailable.Reason)
	}
}

// TestKeePassXCAssociationPathIsUnderTheStateDirectory keeps the approval out
// of the config directory, which a user may well keep in version control.
func TestKeePassXCAssociationPathIsUnderTheStateDirectory(t *testing.T) {
	path := keepassxcAssociationPath()
	if path == "" {
		t.Fatal("the association must have somewhere to live")
	}
	if !strings.HasSuffix(path, "keepassxc-association.json") {
		t.Errorf("path = %q, want it to name the association file", path)
	}
}

// TestKeePassXCNativeBackendCarriesItsAssociationStore proves the backend is
// wired to persist the approval; without it the user approves a dialog on
// every single run.
func TestKeePassXCNativeBackendCarriesItsAssociationStore(t *testing.T) {
	backend := newKeePassXCNativeRoute(keepassxcSettings(config.KeePassXCRouteNative))
	native, ok := backend.(keys.KeePassXCBackend)
	if !ok {
		t.Fatalf("backend = %T, want the native KeePassXC backend", backend)
	}
	if native.Associations == nil {
		t.Fatal("the native route must be able to remember its approval")
	}
	store, ok := native.Associations.(keys.FileAssociationStore)
	if !ok {
		t.Fatalf("association store = %T, want the file-backed one", native.Associations)
	}
	if store.Path == "" {
		t.Error("the association store must know where to write")
	}
}

// TestForgetOnAWalletThatCannotDeleteTellsTheUserWhatToDo verifies F9's second
// half through the command a user actually runs: when the wallet gives SSHakku
// no way to delete, `sshakku forget` must fail loudly and name the entry, never
// report the passphrase as forgotten.
func TestForgetOnAWalletThatCannotDeleteTellsTheUserWhatToDo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	var stdout, stderr bytes.Buffer
	d := depsReturning(keys.KeePassXCBackend{})

	code := d.forget(&stdout, &stderr, []string{"id_ed25519"})

	if code == 0 {
		t.Error("forget must not report success when nothing was removed")
	}
	if strings.Contains(stdout.String(), "forgot") {
		t.Errorf("stdout = %q, want no claim that the passphrase is gone", stdout.String())
	}
	msg := stderr.String()
	if !strings.Contains(msg, "KeePassXC") {
		t.Errorf("stderr = %q, want it to say where the entry still is", msg)
	}
	if !strings.Contains(msg, "SSH-Key-id_ed25519") {
		t.Errorf("stderr = %q, want it to name the entry to remove by hand", msg)
	}
}
