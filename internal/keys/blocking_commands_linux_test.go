//go:build linux

package keys

import (
	"context"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/run"
)

// platformBlockingTools names the programs only a Linux system runs.
//
// Written out rather than taken from the constants the code resolves them by:
// this is the list of programs SSHakku is claimed to run here, and a list
// derived from the implementation would agree with it whatever it became.
func platformBlockingTools() []string {
	return []string{"secret-tool", "kdialog", "zenity", "pinentry", "xset"}
}

// platformBlockingCases adds the programs only a Linux system reaches: the
// libsecret CLI its default wallet is read through, and the X11/KDE pieces the
// graphical passphrase prompt is made of.
//
// The GUI-detection case is left on the bare default on purpose: it witnesses
// the structural net, that a call site choosing no budget still gets a finite
// one. That the defaults themselves are finite is TestTimeoutDefaults.
func platformBlockingCases(ctx context.Context, brief time.Duration) []blockingCase {
	return []blockingCase{
		{"GUI detection (xset)", func() {
			prompt.HasGraphicalSession(ctx, prompt.GUIEnv{Display: ":0"}, run.ExecRunner{})
		}},
		{"graphical passphrase prompt (kdialog)", func() {
			_, _ = prompt.KDialogPrompter{Runner: run.ExecRunner{}, Timeout: brief}.Prompt(ctx, "id_test")
		}},
		{"graphical passphrase prompt (zenity)", func() {
			_, _ = prompt.ZenityPrompter{Runner: run.ExecRunner{}, Timeout: brief}.Prompt(ctx, "id_test")
		}},
		{"graphical passphrase prompt (pinentry)", func() {
			_, _ = prompt.PinentryPrompter{Timeout: brief}.Prompt(ctx, "id_test")
		}},
		{"which pinentry is installed", func() {
			prompt.PinentryPrompter{ProbeTimeout: brief}.Available(ctx)
		}},
		{"secret-tool Lookup", func() {
			_, _, _ = SecretToolBackend{Runner: run.ExecRunner{}, User: "u", Timeout: brief}.Lookup(ctx, defaultServicePrefix+"-id_test")
		}},
		{"secret-tool Store", func() {
			_ = SecretToolBackend{Runner: run.ExecRunner{}, User: "u", Timeout: brief}.Store(ctx, defaultServicePrefix+"-id_test", "label", "s3cret")
		}},
		{"secret-tool Delete", func() {
			_ = SecretToolBackend{Runner: run.ExecRunner{}, User: "u", Timeout: brief}.Delete(ctx, defaultServicePrefix+"-id_test")
		}},
	}
}
