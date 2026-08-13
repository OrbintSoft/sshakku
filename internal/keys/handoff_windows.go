//go:build windows

package keys

import (
	"context"
	"errors"
	"time"
)

// errNoHandoff is what the passphrase handoff reports here. Linux hands the
// passphrase to the askpass child through the kernel keyring and Darwin
// through a private Unix socket; Windows has neither, and the mechanism that
// would replace them — a named pipe with an ACL naming this user alone — is
// not written yet. Failing is the only honest answer: every alternative that
// would compile just as well (an environment variable, a temporary file) is
// one of the places the passphrase must never appear.
var errNoHandoff = errors.New("passphrase handoff is not implemented on windows")

// stashPassphrase reports errNoHandoff.
func stashPassphrase(string, time.Duration) (string, error) { return "", errNoHandoff }

// fetchPassphrase reports errNoHandoff.
func fetchPassphrase(context.Context, string) (string, error) { return "", errNoHandoff }
