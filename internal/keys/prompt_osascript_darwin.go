//go:build darwin

package keys

import (
	_ "embed"
	"os"
	"strings"
	"time"
)

// osascriptBin is macOS's AppleScript interpreter. Unlike the Linux dialog
// tools it is part of the base system, so a Mac that can show a dialog at all
// can show this one, with nothing installed.
const osascriptBin = "osascript"

// passphraseDialog is the dialog itself, kept as AppleScript in a file of its
// own so it can be read, diffed and compiled as AppleScript rather than living
// inside a Go string. It is carried in the binary rather than installed beside
// it: an asset that has to be found at runtime is an asset that can be missing.
//
//go:embed prompt_darwin.applescript
var passphraseDialog string

// OsascriptPrompter prompts with a macOS dialog drawn by AppleScript. The
// typed text comes back on stdout; a dialog the user dismisses exits non-zero.
type OsascriptPrompter struct {
	Runner Runner
	// Timeout bounds the dialog. It is a person's budget, not a machine's, but
	// still finite: a dialog nobody answers must not strand the shell that
	// raised it. Zero selects DefaultInteractiveTimeout.
	Timeout time.Duration
	// lookPath resolves a binary on PATH; nil uses the os/exec default.
	// Injectable for tests.
	lookPath func(string) (string, error)
}

// writeDialogScript materialises the embedded script where osascript can run
// it as a program file, so the key name can be handed over as an argument
// instead of being pasted into the source. The returned cleanup removes it.
//
// The file holds no secret: what the dialog collects is written to osascript's
// stdout, never back into the script.
var writeDialogScript = func() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "sshakku-prompt-*.applescript")
	if err != nil {
		return "", nil, err
	}
	remove := func() { _ = os.Remove(f.Name()) }
	if _, err := f.WriteString(passphraseDialog); err != nil {
		_ = f.Close()
		remove()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		remove()
		return "", nil, err
	}
	return f.Name(), remove, nil
}

// Prompt shows the passphrase dialog for keyname.
func (p OsascriptPrompter) Prompt(keyname string) (string, error) {
	script, cleanup, err := writeDialogScript()
	if err != nil {
		return "", err
	}
	defer cleanup()

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultInteractiveTimeout
	}
	res, err := p.Runner.Run(Cmd{
		Name:    osascriptBin,
		Args:    []string{script, keyname},
		Timeout: timeout,
	})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", ErrPromptCanceled
	}
	return strings.TrimRight(string(res.Stdout), "\n"), nil
}

// Available reports whether osascript is on PATH.
func (p OsascriptPrompter) Available() bool {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	_, err := look(osascriptBin)
	return err == nil
}

var _ Prompter = OsascriptPrompter{}
