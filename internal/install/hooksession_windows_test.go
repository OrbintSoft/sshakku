//go:build windows

package install

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What a re-executed test binary is told when it is standing in for sshakku
// itself, where it writes down how it was called, and which hook the session
// runs.
//
// The hook runs a binary, and what it runs has to be a real executable: a
// script leaves $LASTEXITCODE holding whatever was there before, and the hook
// reads that to decide whether it was answered at all. A test's own binary is
// the one executable it is sure of having.
const (
	standingInForSSHakku  = "SSHAKKU_TEST_STANDING_IN_FOR_SSHAKKU"
	whereToWriteTheCalls  = "SSHAKKU_TEST_CALLS"
	theHookTheSessionRuns = "SSHAKKU_TEST_HOOK"
)

// aLogFileNothingOpens is what the stand-in names as the session log. The hook
// only has to be given one before it will go on; nothing here opens it.
const aLogFileNothingOpens = `C:\sshakku-a-test-never-writes\sessions.log`

// anEndpointNothingServes is what the stand-in names as the agent endpoint. The
// session puts it in its environment and no test connects to it.
const anEndpointNothingServes = `\\.\pipe\sshakku-a-test-never-opens`

func TestMain(m *testing.M) {
	if os.Getenv(standingInForSSHakku) == "" {
		os.Exit(m.Run())
	}
	os.Exit(standInForSSHakku(os.Args[1:]))
}

// standInForSSHakku answers what the hook asks the binary for, and writes down
// every call so that a test can see which of them a session made.
//
// shell-init is answered with assignments printed by the product's own dialect
// rather than with a form invented here: a hook that would not read back what
// the real binary prints is not the hook under test.
func standInForSSHakku(args []string) int {
	if len(args) == 0 {
		return 2
	}
	if err := writeDownTheCall(args); err != nil {
		return 3
	}
	if args[0] != "shell-init" {
		return 0
	}
	dialect, err := shell.Named(shell.PowerShell)
	if err != nil {
		return 4
	}
	if _, err := io.WriteString(os.Stdout,
		dialect.SetVar("agent_sock", anEndpointNothingServes)+
			dialect.SetVar("log_file", aLogFileNothingOpens)); err != nil {
		return 5
	}
	return 0
}

// writeDownTheCall appends one call to the file the test named, whole, so that
// a session making several of them leaves all of them behind.
func writeDownTheCall(args []string) error {
	file, err := os.OpenFile(os.Getenv(whereToWriteTheCalls), //nolint:gosec // G703 follows this path back to the environment; the test put it there and it names a file under the directory the test is given
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := io.WriteString(file, strings.Join(args, " ")+"\n"); err != nil {
		return err
	}
	return file.Close()
}

// sessionShape is one way of opening a PowerShell, in the two parts the product
// reads separately: what the command line says, and where the session reads
// from.
type sessionShape struct {
	// handed is how the session was given its work, "command" or "file".
	handed string
	// stays is whether it was told not to leave when that work is done.
	stays bool
	// mayNotAsk is whether the caller stated outright that nothing may ask.
	mayNotAsk bool
	// atAConsole is whether it reads from a console rather than from a pipe.
	atAConsole bool
}

// theCallsMadeBy opens one session of that shape, has it run a hook rendered
// against the stand-in, and returns every call the hook made.
func theCallsMadeBy(t *testing.T, shape sessionShape) []string {
	t.Helper()

	interpreter, _ := aLiveInterpreter(t)
	work := t.TempDir()
	calls := filepath.Join(work, "calls.txt")

	args := []string{
		"-NoProfile", "-File", inTestdata(t, "open-session.ps1"),
		"-Interpreter", interpreter,
		"-Runner", inTestdata(t, "run-hook.ps1"),
		"-Handed", shape.handed,
	}
	if shape.stays {
		args = append(args, "-Stays")
	}
	if shape.mayNotAsk {
		args = append(args, "-MayNotAsk")
	}
	if shape.atAConsole {
		args = append(args, "-AtAConsole")
	}

	cmd := exec.CommandContext(t.Context(), interpreter, args...)
	cmd.Env = append(os.Environ(),
		standingInForSSHakku+"=1",
		whereToWriteTheCalls+"="+calls,
		theHookTheSessionRuns+"="+aRenderedHook(t, work))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "opening the session: %s", out)

	made, err := os.ReadFile(calls)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	// Line endings are not all one shell's: the stand-in writes its own lines,
	// and the session writes what it read from through this shell's own
	// Add-Content, which ends a line the way this system does.
	lines := strings.Split(strings.TrimSpace(string(made)), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	return lines
}

// aRenderedHook writes the real hook, rendered against the binary standing in
// for sshakku, and returns where it put it.
func aRenderedHook(t *testing.T, dir string) string {
	t.Helper()

	standIn, err := os.Executable()
	require.NoError(t, err)
	dialect, err := shell.Named(shell.PowerShell)
	require.NoError(t, err)
	rendered, err := RenderHook(powerShellHookTemplate, PowerShellBinaryPlaceholder, standIn, dialect)
	require.NoError(t, err)

	hook := filepath.Join(dir, "shell-hook.ps1")
	require.NoError(t, os.WriteFile(hook, rendered, hookMode))
	return hook
}

// whatTheSessionRead is the line the session writes down about where its input
// came from, in that shell's own spelling of a boolean. A test requires it so
// that a machine which could not give a session a console says so, instead of
// having the product reported as failing to load a key.
func whatTheSessionRead(atAConsole bool) string {
	if atAConsole {
		return "input redirected: False"
	}
	return "input redirected: True"
}

// inTestdata names one of the scaffolding scripts in full, since the session
// that runs it is started by another process and inherits nothing of a test's
// idea of where it is.
func inTestdata(t *testing.T, name string) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("testdata", name))
	require.NoError(t, err)
	return path
}

// F60: a shell a terminal opened is yours even when the terminal handed it a
// command of its own to run first, and a session handed work that leaves when
// the work is done is not — whatever kind of console it was given.
//
// Every shape below is opened against the real hook and judged by what the hook
// then did, rather than by what it worked out: a session's keys are loaded by a
// call to the binary or they are not loaded at all.
func TestOnlyASessionThatCouldBeAnsweredLoadsItsKeys(t *testing.T) {
	for _, session := range []struct {
		name      string
		shape     sessionShape
		loadsKeys bool
	}{
		{
			// What VS Code's integrated terminal opens, and what a terminal
			// arranging its own integration opens generally: handed a command
			// of its own, and told to stay for the prompt that follows it.
			name:      "a terminal that hands its shell a command of its own",
			shape:     sessionShape{handed: "command", stays: true, atAConsole: true},
			loadsKeys: true,
		},
		{
			// The same command line without being told to stay: this one runs
			// what it was given and leaves, which is what a build step does.
			name:      "a build step handed a command at a console",
			shape:     sessionShape{handed: "command", stays: false, atAConsole: true},
			loadsKeys: false,
		},
		{
			// A scheduled job is given a console exactly as a terminal window
			// is, which is why the console cannot be the whole of the answer.
			name:      "a scheduled job handed a script at a console",
			shape:     sessionShape{handed: "file", stays: false, atAConsole: true},
			loadsKeys: false,
		},
		{
			// A terminal's own command line means nothing where the session's
			// input comes from a pipe: whatever it asked could not be answered.
			name:      "a terminal's own shape, driven through a pipe",
			shape:     sessionShape{handed: "command", stays: true, atAConsole: false},
			loadsKeys: false,
		},
		{
			// A caller saying outright that nothing may ask is believed, even
			// where everything else about the session says somebody is there.
			name:      "a terminal's own shape, told that nothing may ask",
			shape:     sessionShape{handed: "command", stays: true, mayNotAsk: true, atAConsole: true},
			loadsKeys: false,
		},
	} {
		t.Run(session.name, func(t *testing.T) {
			calls := theCallsMadeBy(t, session.shape)

			require.Contains(t, calls, whatTheSessionRead(session.shape.atAConsole),
				"this session was not opened in the shape this case is about")
			require.Contains(t, calls, "shell-init --shell=powershell",
				"this session never reached the binary at all, so what it did about keys says nothing")
			assert.Equal(t, session.loadsKeys, slices.Contains(calls, "load-keys"),
				"whether this session's keys are loaded")
		})
	}
}
