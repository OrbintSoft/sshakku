// Package runtest provides stand-ins for run.Runner, so a component that drives
// an external program can be tested without one on the machine running the test.
//
// It replaces the process boundary and nothing else: what a test using these
// asserts is still the component's own decisions — which program it chose, what
// argv and standard input it sent, what it made of the answer — with only the
// spawning taken away. The two shapes here differ in what they make easy: Runner
// answers per program name, for a test that stubs several tools independently;
// Recorder answers in call order and keeps every Cmd, for a test whose subject
// is the sequence of calls a component makes.
package runtest

import (
	"context"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// Runner answers Run from a per-binary handler table, so a test can stub
// ssh-keygen, ssh-add, secret-tool, etc. independently and inspect the calls.
// A command with no handler is answered with an error naming it, so a component
// that ran something the test did not expect says so rather than reading a
// zero Result as an answer.
type Runner struct {
	handlers map[string]func(run.Cmd) (run.Result, error)
	// Calls holds every command asked for, in order.
	Calls []run.Cmd
}

// NewRunner returns a Runner with no handlers registered yet.
func NewRunner() *Runner {
	return &Runner{handlers: make(map[string]func(run.Cmd) (run.Result, error))}
}

// On registers a handler for a command name and returns the Runner, so a table
// of stubbed programs can be built in one expression.
func (f *Runner) On(name string, h func(run.Cmd) (run.Result, error)) *Runner {
	f.handlers[name] = h
	return f
}

func (f *Runner) Run(_ context.Context, c run.Cmd) (run.Result, error) {
	f.Calls = append(f.Calls, c)
	if h, ok := f.handlers[c.Name]; ok {
		return h(c)
	}
	return run.Result{}, fmt.Errorf("unexpected command %q", c.Name)
}

// Stdout builds a handler that returns out on stdout with the given exit code.
func Stdout(out string, code int) func(run.Cmd) (run.Result, error) {
	return func(run.Cmd) (run.Result, error) {
		return run.Result{Stdout: []byte(out), Code: code}, nil
	}
}

// Fails builds a handler that reports a failure to start the process — the one
// outcome a Runner reports as an error rather than in Result.Code.
func Fails(err error) func(run.Cmd) (run.Result, error) {
	return func(run.Cmd) (run.Result, error) { return run.Result{}, err }
}

// Recorder records every command it was asked to run and answers from a
// scripted list, in call order. It records what crossed the process boundary,
// so a test can assert on the argv and the standard input rather than on what
// the component meant to send.
//
// Errs takes precedence over Results at the same index; past the end of both, a
// call is answered with a zero Result and no error.
type Recorder struct {
	Results []run.Result
	Errs    []error
	Calls   []run.Cmd
}

func (r *Recorder) Run(_ context.Context, c run.Cmd) (run.Result, error) {
	r.Calls = append(r.Calls, c)
	i := len(r.Calls) - 1
	if i < len(r.Errs) && r.Errs[i] != nil {
		return run.Result{}, r.Errs[i]
	}
	if i < len(r.Results) {
		return r.Results[i], nil
	}
	return run.Result{}, nil
}

var (
	_ run.Runner = (*Runner)(nil)
	_ run.Runner = (*Recorder)(nil)
)
