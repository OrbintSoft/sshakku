package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// forget deletes stored passphrases: either the named keys, or every entry
// sshakku manages with --all. Argument validation happens before any secret
// backend is opened, so a usage error never touches the D-Bus session bus.
func (d deps) forget(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	all := false
	var names []string
	for _, a := range args {
		if a == "--all" {
			all = true
			continue
		}
		names = append(names, a)
	}
	switch {
	case all && len(names) > 0:
		_, _ = fmt.Fprintln(stderr, "sshakku: forget: --all cannot be combined with key names")
		return 2
	case !all && len(names) == 0:
		_, _ = fmt.Fprintln(stderr, "sshakku: forget: specify one or more key names, or --all")
		return 2
	}

	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir)
	log := sessionlog.New(layout.LogFile)
	settings := loadSettings(layout, "forget", log)
	secret, closeSecret := d.newSecret(ctx, currentUser(), log, settings)
	defer closeSecret()
	defer forgetUnlock(ctx, secret, log)()

	services, ok := forgetTargets(ctx, secret, stderr, all, names, settings.ServicePrefix)
	if !ok {
		return 1
	}
	if !forgetEach(ctx, secret, stdout, stderr, log, services) {
		return 1
	}
	return 0
}

// forgetUnlock unlocks the secret store for the whole operation and returns the
// relock to defer. forget always touches the store (listing and/or deleting),
// so — unlike load-keys, which unlocks lazily since some keys may need no wallet
// access at all — it unlocks once up front instead of once per List/Delete call.
// A store that does not lock, or one that refused to unlock, relocks to nothing.
func forgetUnlock(ctx context.Context, secret wallet.Backend, log *sessionlog.Logger) func() {
	sess, ok := secret.(wallet.Session)
	if !ok {
		return func() {}
	}
	if err := sess.Unlock(ctx); err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("forget: unlock secret store: %v", err))
		return func() {}
	}
	return func() {
		if err := sess.Lock(ctx); err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("forget: lock secret store: %v", err))
		}
	}
}

// forgetTargets names the services to delete: everything the store holds for us
// under --all, or one per key named on the command line. It reports false when
// the store could not be listed, having said so on stderr.
func forgetTargets(ctx context.Context, secret wallet.Backend, stderr io.Writer, all bool, names []string, prefix string) ([]string, bool) {
	if !all {
		services := make([]string, len(names))
		for i, name := range names {
			services[i] = prefix + "-" + name
		}
		return services, true
	}
	list, err := secret.List(ctx)
	if err != nil {
		if errors.Is(err, wallet.ErrListUnsupported) {
			_, _ = fmt.Fprintln(stderr, "sshakku: forget --all needs the native Secret Service backend; name keys explicitly instead")
		} else {
			_, _ = fmt.Fprintf(stderr, "sshakku: forget: %v\n", err)
		}
		return nil, false
	}
	return list, true
}

// forgetEach deletes every named service, reporting each failure and carrying
// on: one entry the backend will not part with is not a reason to leave the
// rest stored. It reports whether all of them went.
func forgetEach(ctx context.Context, secret wallet.Backend, stdout, stderr io.Writer, log *sessionlog.Logger, services []string) bool {
	ok := true
	for _, service := range services {
		if err := secret.Delete(ctx, service); err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: forget %s: %v\n", service, err)
			_ = log.Log("ERROR", fmt.Sprintf("forget %s: %v", service, err))
			ok = false
			continue
		}
		_, _ = fmt.Fprintf(stdout, "forgot %s\n", service)
		_ = log.Log("INFO", "forgot "+service)
	}
	return ok
}
