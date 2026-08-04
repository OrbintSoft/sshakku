//go:build linux

package keys

import (
	"strings"
	"time"
)

// kdialogBin is KDE's dialog tool.
const kdialogBin = "kdialog"

// KDialogPrompter prompts via `kdialog --password`. The entered text is returned
// on stdout; a canceled or closed dialog exits non-zero.
type KDialogPrompter struct {
	Runner Runner
	// Timeout bounds the dialog. It is a person's budget, not a machine's, but
	// still finite: a dialog nobody answers must not strand the shell that
	// raised it. Zero selects DefaultInteractiveTimeout.
	Timeout time.Duration
	// lookPath resolves a binary on PATH; nil uses the os/exec default. Injectable
	// for tests.
	lookPath func(string) (string, error)
}

// Prompt shows the password dialog for keyname.
func (p KDialogPrompter) Prompt(keyname string) (string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultInteractiveTimeout
	}
	res, err := p.Runner.Run(Cmd{
		Name:    kdialogBin,
		Args:    []string{"--password", "Enter passphrase for " + keyname},
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

// Name is what to call this prompter in a message.
func (p KDialogPrompter) Name() string { return kdialogBin }

// Available reports whether kdialog is on PATH.
func (p KDialogPrompter) Available() bool {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	_, err := look(kdialogBin)
	return err == nil
}

var _ Prompter = KDialogPrompter{}
