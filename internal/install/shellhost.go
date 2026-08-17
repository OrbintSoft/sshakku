package install

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
)

// queryShellScript asks one Bourne shell about itself. It is a real file rather
// than a string in this source so that it can be read, diffed and linted as the
// language it is written in.
//
//go:embed query-shell.sh
var queryShellScript []byte

// queryShellMode keeps the borrowed copy readable and runnable only by the
// account running the query, which is also the account that will run it.
const queryShellMode fs.FileMode = 0o700

// Shell is what one Bourne shell said about itself. The paths are in that
// shell's own spelling, which on a POSIX-emulating environment is not this
// program's — see Cygpath.
type Shell struct {
	// Home is the directory this shell will look in for its startup files.
	Home string
	// Exe is the interpreter's own path as it knows it. Empty when the shell
	// does not report one, which is not a failure: it is used for saying which
	// shell answered, not for finding anything.
	Exe string
	// Version is what the shell calls itself, for a report to quote.
	Version string
}

// AskShell runs the Bourne shell at exe and returns what it reported about
// itself.
//
// The script is handed over as a named file, in the clear, for the same reasons
// the PowerShell query is — see queryArguments. Only standard output is read,
// so anything a startup file of the user's own prints to standard error cannot
// be mistaken for an answer.
func AskShell(ctx context.Context, exe string) (Shell, error) {
	script, cleanup, err := shellScriptOnDisk()
	if err != nil {
		return Shell{}, fmt.Errorf("laying out the query for %s: %w", exe, err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, exe, script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return Shell{}, fmt.Errorf("asking %s about itself: %w%s", exe, err, explain(stderr.String()))
	}

	shell, err := parseShell(stdout)
	if err != nil {
		return Shell{}, fmt.Errorf("reading what %s said about itself: %w%s", exe, err, explain(stderr.String()))
	}
	return shell, nil
}

func shellScriptOnDisk() (string, func(), error) {
	return scriptOnDisk("query-shell.sh", queryShellScript, queryShellMode)
}

// parseShell reads the key=value lines the query prints.
//
// A key nobody here knows is ignored rather than refused, so that the script
// may answer more than this version asks. A home directory, on the other hand,
// is the whole point: without it there is no file to wire, and reporting that
// as a shell whose home is the empty string would wire the current directory.
func parseShell(stdout []byte) (Shell, error) {
	if len(bytes.TrimSpace(stdout)) == 0 {
		return Shell{}, errors.New("it printed nothing")
	}

	var shell Shell
	for line := range strings.Lines(strings.ReplaceAll(string(stdout), "\r\n", "\n")) {
		key, value, found := strings.Cut(strings.TrimSuffix(line, "\n"), "=")
		if !found {
			continue
		}
		switch key {
		case "home":
			shell.Home = value
		case "shell":
			shell.Exe = value
		case "version":
			shell.Version = value
		}
	}

	if shell.Home == "" {
		return Shell{}, errors.New("it named no home directory, so there is nothing to wire")
	}
	return shell, nil
}
