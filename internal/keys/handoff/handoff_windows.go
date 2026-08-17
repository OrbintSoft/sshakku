//go:build windows

package handoff

import (
	"context"
	"time"

	"github.com/OrbintSoft/sshakku/internal/platform"
)

// errNoHandoff is what the passphrase handoff reports here. Linux hands the
// passphrase to the askpass child through the kernel keyring and Darwin
// through a private Unix socket; Windows has neither, and the mechanism that
// would replace them — a named pipe with an ACL naming this user alone — is
// not written yet. Failing is the only honest answer: every alternative that
// would compile just as well (an environment variable, a temporary file) is
// one of the places the passphrase must never appear.
var errNoHandoff = platform.Unimplemented("passphrase handoff")

// Stash reports errNoHandoff.
func Stash(string, time.Duration) (string, error) { return "", errNoHandoff }

// Fetch reports errNoHandoff.
func Fetch(context.Context, string) (string, error) { return "", errNoHandoff }
