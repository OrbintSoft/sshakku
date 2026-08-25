//go:build linux

package wallet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// secretToolBin is the libsecret CLI shared by KDE Wallet and GNOME Keyring,
// which both implement the D-Bus Secret Service API.
const secretToolBin = "secret-tool"

// SecretTool keeps passphrases in the D-Bus Secret Service via secret-tool.
// The passphrase travels on the process's stdin, never in argv, so it cannot leak
// through `ps` or /proc/<pid>/cmdline.
type SecretTool struct {
	Runner run.Runner
	// User is the "username" attribute, constant for the login session.
	User string
	// Timeout bounds each secret-tool call. The wallet may be locked behind an
	// unlock prompt that nobody can answer, and something is waiting on the
	// answer — a login shell, or an ssh at a passphrase prompt — so the wait is
	// finite and the caller falls back to asking on the terminal. Zero selects
	// run.DefaultCommandTimeout.
	Timeout time.Duration
}

// run bounds every secret-tool call.
func (b SecretTool) run(ctx context.Context, c run.Cmd) (run.Result, error) {
	if c.Timeout <= 0 {
		c.Timeout = b.Timeout
	}
	return b.Runner.Run(ctx, c)
}

// Lookup runs `secret-tool lookup service <service> username <user>`. secret-tool
// emits the secret verbatim, so a trailing newline (e.g. from an entry stored by
// the earlier shell version) is trimmed. A non-zero exit means no entry — handled
// as a miss, not an error, so the loader falls back to prompting.
func (b SecretTool) Lookup(ctx context.Context, service string) (string, bool, error) {
	res, err := b.run(ctx, run.Cmd{
		Name: secretToolBin,
		Args: []string{"lookup", secretAttrService, service, secretAttrUsername, b.User},
	})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		return "", false, nil
	}
	return strings.TrimRight(string(res.Stdout), "\n"), true, nil
}

// Store runs `secret-tool store --label=<label> service <service> username
// <user>`, feeding the passphrase on stdin. Unlike the earlier `echo | …`, no
// trailing newline is appended, so the secret is stored exactly.
func (b SecretTool) Store(ctx context.Context, service, label, passphrase string) error {
	res, err := b.run(ctx, run.Cmd{
		Name:  secretToolBin,
		Args:  []string{"store", "--label=" + label, secretAttrService, service, secretAttrUsername, b.User},
		Stdin: passphrase,
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("secret-tool store exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Delete runs `secret-tool clear service <service> username <user>`. Known
// caveat from manual testing: secret-tool clear has been observed to exit 1
// with no stderr against a real, present entry — a silent failure to remove
// it — so a non-zero exit here is reported but should not be over-trusted as
// proof the entry is gone; there is no native-D-Bus alternative on this
// fallback path (it exists only because the D-Bus session itself was
// unreachable).
func (b SecretTool) Delete(ctx context.Context, service string) error {
	res, err := b.run(ctx, run.Cmd{
		Name: secretToolBin,
		Args: []string{"clear", secretAttrService, service, secretAttrUsername, b.User},
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("secret-tool clear exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// List always fails: secret-tool has no verb to enumerate entries without
// already knowing their exact attributes, so "forget everything" is only
// supported through SecretService's native D-Bus enumeration.
func (b SecretTool) List(ctx context.Context) ([]string, error) {
	return nil, ErrListUnsupported
}

var _ Backend = SecretTool{}
