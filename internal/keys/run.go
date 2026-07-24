package keys

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// ExecRunner runs commands via os/exec, capturing stdout and stderr.
type ExecRunner struct{}

// Run starts c, feeds Stdin if set, appends Env to the inherited environment, and
// returns the captured output with the exit code. A non-zero exit is reported in
// Result.Code with a nil error; only a failure to start the process is an error. A
// positive c.Timeout kills the process once it elapses — a killed process is a
// signaled exit, not a failure to start, so it also surfaces as a Result with no
// error, the same as any other non-zero exit.
func (ExecRunner) Run(c Cmd) (Result, error) {
	ctx := context.Background()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
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

// execLookPath resolves a binary on PATH; it is a variable so tests can stub the
// PATH lookup.
var execLookPath = exec.LookPath
