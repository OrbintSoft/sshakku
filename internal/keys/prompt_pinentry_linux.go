//go:build linux

package keys

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// pinentryBin is the passphrase dialog that comes with GnuPG. Which toolkit it
// draws with — Qt, GTK, GNOME — is settled by the distribution's own choice of
// pinentry, so asking for it by this name gets a dialog that suits the desktop
// it appears on, without SSHakku having to recognise the desktop.
const pinentryBin = "pinentry"

// The Assuan error codes a dismissed dialog reports. Only the low 16 bits of an
// error number are the code itself; the rest names the component that raised it.
const (
	gpgErrCodeMask      = 0xFFFF
	gpgErrCanceled      = 99
	gpgErrFullyCanceled = 100
)

// PinentryPrompter prompts via pinentry. Unlike a dialog that takes its
// arguments on a command line and prints the answer, pinentry is driven through
// a conversation on its own stdin and stdout: the description and the prompt are
// set with one request each, the passphrase is asked for with another, and every
// request is answered before the next is sent.
type PinentryPrompter struct {
	// Bin is the program to run. Empty means pinentry, found on PATH.
	Bin string
	// Timeout bounds the dialog. It is a person's budget, not a machine's, but
	// still finite: a dialog nobody answers must not strand the shell that
	// raised it. Zero selects DefaultInteractiveTimeout.
	Timeout time.Duration
	// lookPath resolves a binary on PATH; nil uses the os/exec default. Injectable
	// for tests.
	lookPath func(string) (string, error)
}

// Prompt asks for keyname's passphrase and returns what was typed into the
// dialog, or ErrPromptCanceled if it was dismissed.
//
// pinentry inherits this process's environment, which is how it finds the
// display to draw on. It is deliberately not told about a terminal: a pinentry
// built for the console would otherwise take one over, and the terminal is the
// other prompter's job, not this one's.
func (p PinentryPrompter) Prompt(keyname string) (string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultInteractiveTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.bin())
	// The deadline has to end the wait, not merely the process: a dialog that
	// left a child behind keeps the pipe this conversation is read from open,
	// and the read would outlast the budget by however long that child lives.
	cmd.WaitDelay = commandWaitDelay
	boundToProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting %s: %w", p.bin(), err)
	}
	// However the conversation ends, the dialog is closed and the process
	// reaped: one nobody answered must not outlive the shell that raised it.
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	return (&assuanConv{w: stdin, r: bufio.NewReader(stdout)}).getpin(keyname)
}

// Available reports whether pinentry is on PATH.
func (p PinentryPrompter) Available() bool {
	look := p.lookPath
	if look == nil {
		look = execLookPath
	}
	_, err := look(p.bin())
	return err == nil
}

// Name is what to call this prompter in a message.
func (p PinentryPrompter) Name() string { return p.bin() }

// bin is the program to run, defaulted.
func (p PinentryPrompter) bin() string {
	if p.Bin != "" {
		return p.Bin
	}
	return pinentryBin
}

// assuanConv is one conversation with a running pinentry.
type assuanConv struct {
	w io.Writer
	r *bufio.Reader
}

// getpin runs the exchange that ends in a passphrase: pinentry announces itself,
// is told what to say and what to ask, and is then asked for the answer.
func (c *assuanConv) getpin(keyname string) (string, error) {
	if _, err := c.reply(); err != nil {
		return "", fmt.Errorf("pinentry greeting: %w", err)
	}
	for _, req := range []string{
		"SETTITLE " + assuanEncode("SSHakku"),
		"SETDESC " + assuanEncode("Enter passphrase for "+keyname),
		"SETPROMPT " + assuanEncode("Passphrase:"),
	} {
		if _, err := c.send(req); err != nil {
			return "", err
		}
	}
	pass, err := c.send("GETPIN")
	if err != nil {
		return "", err
	}
	// Best effort: the answer is already in hand, and a pinentry that will not
	// say goodbye is still killed and reaped by the caller.
	_, _ = c.send("BYE")
	return pass, nil
}

// send writes one request and returns what it was answered with.
func (c *assuanConv) send(req string) (string, error) {
	if _, err := io.WriteString(c.w, req+"\n"); err != nil {
		return "", fmt.Errorf("writing to pinentry: %w", err)
	}
	return c.reply()
}

// reply reads response lines until the request is answered. Data lines carry the
// answer; status lines and comments are pinentry talking about itself and answer
// nothing, so they are read past rather than mistaken for a passphrase.
func (c *assuanConv) reply() (string, error) {
	var data strings.Builder
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("pinentry ended the conversation: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "OK" || strings.HasPrefix(line, "OK "):
			return data.String(), nil
		case strings.HasPrefix(line, "ERR "):
			return "", assuanError(strings.TrimPrefix(line, "ERR "))
		case strings.HasPrefix(line, "D "):
			data.WriteString(assuanDecode(strings.TrimPrefix(line, "D ")))
		}
	}
}

// assuanError turns an ERR line into an error, recognising the one that is not a
// failure at all: a dialog the user closed on purpose.
func assuanError(rest string) error {
	number, desc, _ := strings.Cut(rest, " ")
	code, err := strconv.Atoi(number)
	if err != nil {
		return fmt.Errorf("pinentry: %s", rest)
	}
	switch code & gpgErrCodeMask {
	case gpgErrCanceled, gpgErrFullyCanceled:
		return ErrPromptCanceled
	}
	if desc == "" {
		return fmt.Errorf("pinentry: error %d", code)
	}
	return fmt.Errorf("pinentry: %s", desc)
}

// assuanDecode undoes the percent-escaping the protocol requires of any byte
// that would otherwise end a line or start an escape. An escape that is not one
// is left as it stands: a passphrase is returned as it was typed, and guessing
// at a malformed one would be guessing at somebody's secret.
func assuanDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// assuanEncode escapes what a request line cannot carry literally.
func assuanEncode(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return r.Replace(s)
}

var _ Prompter = PinentryPrompter{}
