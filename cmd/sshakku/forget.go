package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// forget deletes stored passphrases: either the named keys, or every entry
// sshakku manages with --all. Argument validation happens before any secret
// backend is opened, so a usage error never touches the D-Bus session bus.
func (d deps) forget(stdout, stderr io.Writer, args []string) int {
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
	secret, closeSecret := d.newSecret(currentUser(), log, settings)
	defer closeSecret()

	// forget always touches the secret store (listing and/or deleting), so —
	// unlike load-keys, which unlocks lazily since some keys may need no wallet
	// access at all — it unlocks once up front for the whole operation instead
	// of once per List/Delete call.
	if sess, ok := secret.(keys.SecretSession); ok {
		if err := sess.Unlock(); err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("forget: unlock secret store: %v", err))
		} else {
			defer func() {
				if err := sess.Lock(); err != nil {
					_ = log.Log("ERROR", fmt.Sprintf("forget: lock secret store: %v", err))
				}
			}()
		}
	}

	var services []string
	if all {
		list, err := secret.List()
		if err != nil {
			if errors.Is(err, keys.ErrListUnsupported) {
				_, _ = fmt.Fprintln(stderr, "sshakku: forget --all needs the native Secret Service backend; name keys explicitly instead")
			} else {
				_, _ = fmt.Fprintf(stderr, "sshakku: forget: %v\n", err)
			}
			return 1
		}
		services = list
	} else {
		services = make([]string, len(names))
		for i, name := range names {
			services[i] = keys.DefaultServicePrefix + "-" + name
		}
	}

	fail := false
	for _, service := range services {
		if err := secret.Delete(service); err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: forget %s: %v\n", service, err)
			_ = log.Log("ERROR", fmt.Sprintf("forget %s: %v", service, err))
			fail = true
			continue
		}
		_, _ = fmt.Fprintf(stdout, "forgot %s\n", service)
		_ = log.Log("INFO", fmt.Sprintf("forgot %s", service))
	}
	if fail {
		return 1
	}
	return 0
}
