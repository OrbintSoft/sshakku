//go:build linux

package keys

import (
	"strings"
	"time"
)

// zenityBin is GNOME's dialog tool, the one a GTK desktop is likely to have.
const zenityBin = "zenity"

// ZenityPrompter prompts via `zenity --password`. The entered text is returned
// on stdout; a canceled or closed dialog exits non-zero.
type ZenityPrompter struct {
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
func (p ZenityPrompter) Prompt(keyname string) (string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultInteractiveTimeout
	}
	res, err := p.Runner.Run(Cmd{
		Name: zenityBin,
		// zenity has no argument for the text above the field, so the key being
		// asked about goes in the title, which is the only place it can be read.
		Args:    []string{"--password", "--title", "Enter passphrase for " + keyname},
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
func (p ZenityPrompter) Name() string { return zenityBin }

// Available reports whether zenity is on PATH.
func (p ZenityPrompter) Available() bool {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	_, err := look(zenityBin)
	return err == nil
}

var _ Prompter = ZenityPrompter{}
