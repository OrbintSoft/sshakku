// Package testproc gives a test a real child process to run, for the few tests
// whose subject is the process boundary itself.
//
// Something that spawns processes cannot be tested against a stand-in for
// spawning — that would assert away the very thing under test — so such a test
// needs a real program. It does not need a *particular* program, and reaching
// for sh, cat or sleep makes the test about which tools the machine happens to
// carry rather than about the code: those are absent from a Windows PATH, and
// the test then fails somewhere the code is fine. The program used here is the
// test binary itself, re-entered in a mode that does what the case needs, so
// the same test runs the same way on every system.
//
// Call Serve as the first statement of TestMain, and build commands with
// Command.
package testproc

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"
)

// marker is the first argument that tells a re-executed test binary that it is
// the child and not the test run. It is unlikely enough to collide with a
// testing flag that no escaping scheme is needed.
const marker = "-sshakku-testproc"

// Modes a child can be asked to perform.
const (
	// Emit writes its first argument to standard output and its second to
	// standard error, then exits with the code named by its third.
	Emit = "emit"
	// Sleep waits for the duration named by its first argument, then exits 0.
	// It is what a command that outlives its budget is made of.
	Sleep = "sleep"
	// EchoStdin copies its standard input to its standard output, which is how
	// a test sees what a program was actually handed there.
	EchoStdin = "echo-stdin"
	// EchoEnv prints the value of each environment variable it is named, one
	// per line, empty line included when a variable is unset.
	EchoEnv = "echo-env"
)

// Serve runs the child side and never returns when this process is one. In an
// ordinary test run it returns immediately, having done nothing.
//
// It has to come before the testing package parses flags, since the arguments
// below are not flags it would recognise.
func Serve() {
	// This process's own argv and its exit are what the function is: there is
	// no seam left to observe them from inside, and a test that had one would
	// not be testing this. What decides the child's behaviour is act, which is
	// tested directly, and the round trip through a real child is exercised by
	// this package's own test.
	//coverage:ignore
	if len(os.Args) < 2 || os.Args[1] != marker {
		return
	}
	os.Exit(act(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
}

// Command names this test binary and the arguments that put it in the given
// mode, for a caller to run however it runs things.
func Command(tb testing.TB, mode string, args ...string) (name string, argv []string) {
	tb.Helper()

	exe, err := os.Executable()
	if err != nil {
		tb.Fatalf("locating this test binary, which is the program these tests run: %v", err)
	}
	return exe, append([]string{marker, mode}, args...)
}

// act performs one mode and returns the exit code the child should carry. It
// takes its streams rather than reaching for the process's own, so that what
// each mode does can be checked without spawning anything.
func act(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		say(stderr, "testproc: no mode given\n")
		return 2
	}
	mode, args := argv[0], argv[1:]

	switch mode {
	case Emit:
		if len(args) != 3 {
			return misuse(stderr, mode, "<stdout> <stderr> <exit code>")
		}
		code, err := strconv.Atoi(args[2])
		if err != nil {
			return misuse(stderr, mode, "<stdout> <stderr> <exit code>")
		}
		say(stdout, "%s", args[0])
		say(stderr, "%s", args[1])
		return code

	case Sleep:
		if len(args) != 1 {
			return misuse(stderr, mode, "<duration>")
		}
		d, err := time.ParseDuration(args[0])
		if err != nil {
			return misuse(stderr, mode, "<duration>")
		}
		time.Sleep(d)
		return 0

	case EchoStdin:
		if _, err := io.Copy(stdout, stdin); err != nil {
			say(stderr, "testproc: copying standard input: %v\n", err)
			return 1
		}
		return 0

	case EchoEnv:
		for _, name := range args {
			say(stdout, "%s\n", os.Getenv(name))
		}
		return 0

	default:
		say(stderr, "testproc: no such mode %q\n", mode)
		return 2
	}
}

func misuse(stderr io.Writer, mode, usage string) int {
	say(stderr, "testproc: %s takes %s\n", mode, usage)
	return 2
}

// say writes to one of the child's streams. A stream this process cannot write
// to leaves it nothing to report the failure on and no one to report it to, so
// the error is dropped here rather than at each of the call sites above; the
// test that spawned the child sees the truncated output and fails on that.
func say(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
