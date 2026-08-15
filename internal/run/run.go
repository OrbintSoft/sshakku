// Package run runs the external programs SSHakku drives — ssh-add, ssh-keygen,
// a wallet's CLI, a passphrase dialog — and bounds how long anything outside
// this process may keep a caller waiting.
//
// Every call is given a budget, because a login shell and an `ssh` stopped at a
// passphrase prompt both sit behind these programs: one that neither answers nor
// fails has to become an error the caller can fall back from, rather than
// silence with no end. Runner is the seam that boundary is crossed through, so
// what drives an external tool can be tested without one.
package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Cmd describes one external command invocation. Env entries are appended to the
// current environment; Stdin, when non-empty, is fed to the process — the way a
// passphrase reaches secret-tool without ever appearing in argv. Timeout, when
// positive, bounds how long the process may run before it is killed; zero means
// unbounded, the right default for a call that waits on a human (e.g. a GUI
// unlock prompt) rather than a plain status query.
type Cmd struct {
	Name    string
	Args    []string
	Stdin   string
	Env     []string
	Timeout time.Duration
}

// Result is the outcome of running a Cmd. A non-zero Code is reported here, not
// as an error: callers distinguish meaningful exit codes (e.g. ssh-add -l exits 1
// for an empty agent, secret-tool exits non-zero for a miss) from a failure to
// start the process, which is returned as the error.
type Result struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

// Runner runs an external command and returns its result. It is the seam that
// lets a component driving ssh-add, ssh-keygen, a wallet's CLI or a dialog be
// tested without spawning any of them.
type Runner interface {
	Run(ctx context.Context, c Cmd) (Result, error)
}

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

// BoundToDeadline prepares cmd so that its context's deadline gives control
// back to the caller, and not merely to the process table. It is for a caller
// that drives a process itself rather than through a Runner — one holding a
// conversation with a dialog over its pipes, which Run's single-shot shape
// cannot express. Run applies it to everything it starts.
//
// Two things are needed and neither suffices alone. Killing the process leaves
// whatever it spawned holding the output pipe open, so the group is what gets
// killed; and a pipe still held open outlasts the kill, so WaitDelay bounds the
// second wait as well. A caller that applies one and forgets the other has the
// unbounded wait back.
func BoundToDeadline(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	boundToProcessGroup(cmd)
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
	BoundToDeadline(cmd)
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
