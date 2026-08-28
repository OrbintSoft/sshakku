//go:build windows

package prompt

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// powerShellPrompterName is what gui_prompter calls this dialog, and what a
// message about it names. It names the host that draws the box, as the dialogs
// on the other platforms are named after the program that draws them.
const powerShellPrompterName = "powershell"

// powerShellHosts are the interpreters that can draw the box, most preferred
// first. Both draw it; the order is what each costs to reach a window, and the
// second is the one every Windows has whether anything was installed or not.
var powerShellHosts = []string{"pwsh.exe", "powershell.exe"}

// The exit codes the script itself chooses. Everything else is the host's own —
// a policy that will not load the file, an edition with no WinForms — and is a
// failure to ask rather than an answer. Reading one of those as a dismissal
// would turn a machine where the window never draws into a user who closed it,
// and the asking would stop with nothing having appeared on screen.
const (
	dialogAnswered  = 0
	dialogDismissed = 2
)

// passphraseDialog is the dialog itself, kept as PowerShell in a file of its
// own so it can be read, diffed and linted as PowerShell rather than living
// inside a Go string. It is carried in the binary rather than installed beside
// it: an asset that has to be found at runtime is an asset that can be missing.
//
//go:embed passphrase_windows.ps1
var passphraseDialog string

// errNoPowerShellHost is a machine with no interpreter to draw the box. It
// completes a sentence beginning with the prompter's name.
var errNoPowerShellHost = errors.New("has no PowerShell host installed to draw it")

// errDialogNeverDrew is a host that came back with a code that is neither the
// answer nor the dismissal, which means no window was ever put on the screen.
// It is a value a caller can match on rather than a sentence only a person can
// read: the difference between this and a dismissal decides whether the asking
// carries on, so it must not be something matched by reading the text.
var errDialogNeverDrew = errors.New("the passphrase box did not appear")

// PowerShellPrompter prompts with a window drawn by a PowerShell host. The
// typed text comes back on stdout; a box the user dismisses exits with a code
// of its own, and any other code means the window was never drawn.
type PowerShellPrompter struct {
	Runner run.Runner
	// Timeout bounds the dialog. It is a person's budget, not a machine's, but
	// still finite: a box nobody answers must not strand the shell that raised
	// it. Zero selects run.DefaultInteractiveTimeout.
	Timeout time.Duration
	// lookPath resolves a binary on PATH; nil uses the os/exec default.
	// Injectable for tests.
	lookPath func(string) (string, error)
}

// createDialogScript creates the file the script is written to. The name ends
// in .ps1 because a host runs a script by that name and no other, and it is
// random and created exclusively, because a predictable path in a shared
// temporary directory is one another user can get there first with: what the
// host then runs would be theirs, as us.
//
// It is a variable so a test can hand back a file that cannot be written to,
// which a real temporary directory will not do on request.
var createDialogScript = func() (*os.File, error) {
	return os.CreateTemp("", "sshakku-prompt-*.ps1")
}

// writeDialogScript materialises the embedded script where a host can run it as
// a program file, so the key name can be handed over as an argument instead of
// being pasted into the source. The returned cleanup removes it.
//
// The file holds no secret: what the box collects goes to the host's stdout,
// never back into the script.
func writeDialogScript() (path string, cleanup func(), err error) {
	f, err := createDialogScript()
	if err != nil {
		return "", nil, err
	}
	// Closing is deferred and its error dropped: os.File writes go straight to
	// the kernel, so once WriteString has returned the script is already where
	// the host will read it, and there is nothing left for Close to lose.
	defer func() { _ = f.Close() }()

	remove := func() { _ = os.Remove(f.Name()) }
	if _, err := f.WriteString(passphraseDialog); err != nil {
		remove()
		return "", nil, err
	}
	return f.Name(), remove, nil
}

// Prompt shows the passphrase box for keyname.
func (p PowerShellPrompter) Prompt(ctx context.Context, keyname string) (string, error) {
	host, err := p.host()
	if err != nil {
		return "", err
	}

	script, cleanup, err := writeDialogScript()
	if err != nil {
		return "", err
	}
	defer cleanup()

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = run.DefaultInteractiveTimeout
	}
	res, err := p.Runner.Run(ctx, run.Cmd{
		Name: host,
		// -STA is what a window wants of the thread that draws it, and
		// -NoProfile keeps a profile from having anything to say on standard
		// output, where the passphrase is about to travel.
		Args:    []string{"-NoProfile", "-NonInteractive", "-STA", "-File", script, keyname},
		Timeout: timeout,
	})
	if err != nil {
		return "", err
	}

	switch res.Code {
	case dialogAnswered:
		// Handed back exactly as typed: nothing is trimmed, because a
		// passphrase may end in a space and only its owner knows whether it
		// does. The script writes the text with no line ending after it.
		return string(res.Stdout), nil
	case dialogDismissed:
		return "", ErrCanceled
	default:
		return "", fmt.Errorf("%w: %s exited %d: %s", errDialogNeverDrew, host, res.Code, firstLine(res.Stderr))
	}
}

// host is the interpreter to draw with, or the reason there is none.
func (p PowerShellPrompter) host() (string, error) {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	for _, h := range powerShellHosts {
		if _, err := look(h); err == nil {
			return h, nil
		}
	}
	return "", errNoPowerShellHost
}

// firstLine is the first thing the host said on standard error, which is what
// makes a refusal something the user can act on: "running scripts is disabled
// on this system" is the difference between a dialog nobody can fix and one a
// policy turned off. One line, because it is going into a log line.
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "it said nothing about why"
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Name is what to call this prompter in a message.
func (p PowerShellPrompter) Name() string { return powerShellPrompterName }

// Available reports whether a host is installed to draw the box. Whether the
// session it would draw on has a screen is a different question, and not this
// type's to answer.
func (p PowerShellPrompter) Available(context.Context) bool {
	_, err := p.host()
	return err == nil
}

// WhyUnavailable completes a sentence about this prompter that begins with its
// name. The usual "is not installed" would name the dialog rather than the
// thing that is missing, and there are two hosts either of which would do.
func (p PowerShellPrompter) WhyUnavailable() string { return errNoPowerShellHost.Error() }

var _ Prompter = PowerShellPrompter{}
