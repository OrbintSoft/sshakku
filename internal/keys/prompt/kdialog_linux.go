//go:build linux

package prompt

import (
	"context"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// kdialogBin is KDE's dialog tool.
const kdialogBin = "kdialog"

// KDialogPrompter prompts via `kdialog --password`. The entered text is returned
// on stdout; a canceled or closed dialog exits non-zero.
type KDialogPrompter struct {
	Runner run.Runner
	// Timeout bounds the dialog. It is a person's budget, not a machine's, but
	// still finite: a dialog nobody answers must not strand the shell that
	// raised it. Zero selects run.DefaultInteractiveTimeout.
	Timeout time.Duration
	// lookPath resolves a binary on PATH; nil uses the os/exec default. Injectable
	// for tests.
	lookPath func(string) (string, error)
}

// Prompt shows the password dialog for keyname.
func (p KDialogPrompter) Prompt(ctx context.Context, keyname string) (string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = run.DefaultInteractiveTimeout
	}
	res, err := p.Runner.Run(ctx, run.Cmd{
		Name:    kdialogBin,
		Args:    []string{"--password", "Enter passphrase for " + keyname},
		Timeout: timeout,
	})
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", ErrCanceled
	}
	return strings.TrimRight(string(res.Stdout), "\n"), nil
}

// Name is what to call this prompter in a message.
func (p KDialogPrompter) Name() string { return kdialogBin }

// Available reports whether kdialog is on PATH.
func (p KDialogPrompter) Available(context.Context) bool {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	_, err := look(kdialogBin)
	return err == nil
}

var _ Prompter = KDialogPrompter{}
