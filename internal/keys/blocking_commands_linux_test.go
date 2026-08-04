//go:build linux

package keys

import "time"

// platformBlockingTools names the programs only a Linux system runs.
func platformBlockingTools() []string {
	return []string{secretToolBin, kdialogBin, zenityBin, pinentryBin, "xset"}
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
			HasGraphicalSession(GUIEnv{Display: ":0"}, ExecRunner{})
		}},
		{"graphical passphrase prompt (kdialog)", func() {
			_, _ = KDialogPrompter{Runner: ExecRunner{}, Timeout: brief}.Prompt("id_test")
		}},
		{"graphical passphrase prompt (zenity)", func() {
			_, _ = ZenityPrompter{Runner: ExecRunner{}, Timeout: brief}.Prompt("id_test")
		}},
		{"graphical passphrase prompt (pinentry)", func() {
			_, _ = PinentryPrompter{Timeout: brief}.Prompt("id_test")
		}},
		{"which pinentry is installed", func() {
			PinentryPrompter{ProbeTimeout: brief}.Available()
		}},
		{"secret-tool Lookup", func() {
			_, _, _ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Lookup(defaultServicePrefix + "-id_test")
		}},
		{"secret-tool Store", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Store(defaultServicePrefix+"-id_test", "label", "s3cret")
		}},
		{"secret-tool Delete", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Delete(defaultServicePrefix + "-id_test")
		}},
	}
}
