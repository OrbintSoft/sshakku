//go:build linux

package keys

import "time"

// platformBlockingTools names the programs only a Linux system runs.
func platformBlockingTools() []string {
	return []string{secretToolBin, kdialogBin, "xset"}
}

// platformBlockingCases adds the programs only a Linux system reaches: the
// libsecret CLI its default wallet is read through, and the X11/KDE pieces the
// graphical passphrase prompt is made of.
//
// The GUI-detection case is left on the bare default on purpose: it witnesses
// the structural net, that a call site choosing no budget still gets a finite
// one. That the defaults themselves are finite is TestTimeoutDefaults.
func platformBlockingCases(brief time.Duration) []blockingCase {
	return []blockingCase{
		{"GUI detection (xset)", func() {
			GUIAvailable(GUIEnv{Display: ":0"}, ExecRunner{}, KDialogPrompter{})
		}},
		{"graphical passphrase prompt (kdialog)", func() {
			_, _ = KDialogPrompter{Runner: ExecRunner{}, Timeout: brief}.Prompt("id_test")
		}},
		{"secret-tool Lookup", func() {
			_, _, _ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Lookup("SSH-Key-id_test")
		}},
		{"secret-tool Store", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Store("SSH-Key-id_test", "label", "s3cret")
		}},
		{"secret-tool Delete", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Delete("SSH-Key-id_test")
		}},
	}
}
