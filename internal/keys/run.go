package keys

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultCommandTimeout bounds a command whose caller chose no budget of its
// own. It exists so that no external program can hold a shell up with no end:
// a login shell and an `ssh` waiting on a passphrase both sit behind these
// calls, and a tool that neither answers nor fails — a wallet locked behind a
// prompt nobody can answer, a CLI waiting on a network that has gone away —
// must become an error the caller can fall back from.
const DefaultCommandTimeout = 10 * time.Second

// DefaultInteractiveTimeout bounds a command that is waiting on a person: a
// password dialog, or a CLI that defers to a desktop app for approval. It is
// generous because the limit is human, not mechanical; it is still finite,
// because an unanswered dialog must not strand the shell either.
const DefaultInteractiveTimeout = 2 * time.Minute

// commandWaitDelay is how long a killed command's output is still collected
// before its pipes are closed and the wait ends regardless. Short: by this
// point the command has already been killed for overrunning its budget, and
// what is being waited on is whatever it left running.
const commandWaitDelay = time.Second

// ExecRunner runs commands via os/exec, capturing stdout and stderr.
type ExecRunner struct {
	// Timeout bounds every command this runner starts that does not carry a
	// Cmd.Timeout of its own. Zero selects DefaultCommandTimeout; there is no
	// value meaning "wait forever".
	Timeout time.Duration
}

// Run starts c, feeds Stdin if set, appends Env to the inherited environment, and
// returns the captured output with the exit code. A non-zero exit is reported in
// Result.Code with a nil error; only a failure to start the process is an error.
//
// Every command is bounded: c.Timeout when it sets one, else the runner's
// Timeout, else DefaultCommandTimeout. Once it elapses the process is killed —
// a signaled exit, not a failure to start, so it surfaces as a Result with no
// error, the same as any other non-zero exit, and the caller falls back exactly
// as it would from a tool that refused.
//
// Killing the process is not on its own enough to get control back: a tool that
// left a child behind — a helper, an unlock agent — keeps the output pipe open,
// and the wait then lasts as long as that child does, however dead the tool
// itself is. WaitDelay bounds that second wait too, so the deadline holds for
// what the caller actually experiences rather than only for the process table.
func (r ExecRunner) Run(ctx context.Context, c Cmd) (Result, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = r.Timeout
	}
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.WaitDelay = commandWaitDelay
	boundToProcessGroup(cmd)
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	if c.Env != nil {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := Result{Stdout: out.Bytes(), Stderr: errBuf.Bytes()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}

var _ Runner = ExecRunner{}
