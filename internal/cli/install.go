package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/install"
)

// installFlags is what the two wiring commands take, and whether each carries a
// value. The name is looked up here before any value is consumed, so a flag
// that is not one is reported as that rather than as one missing its value.
var installFlags = map[string]bool{
	"--shell":     true,
	"--shell-exe": true,
	"--profile":   true,
	"--scope":     true,
	"--hosts":     true,
	"--no-path":   false,
}

// The names the two wiring commands answer to. Each is carried in the wiring
// the command builds and read back where the report has to tell which of the
// two ran, so the name a user types and the name the report reasons about are
// one string.
const (
	installCmdName   = "install"
	uninstallCmdName = "uninstall"
)

// install wires the login hook into one shell.
func (d deps) install(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	return d.wire(ctx, stdout, stderr, wiring{
		name:     installCmdName,
		headline: "wired the sshakku hook into one shell:",
		apply:    install.Install,
	}, args)
}

// uninstall takes that wiring back out.
func (d deps) uninstall(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	return d.wire(ctx, stdout, stderr, wiring{
		name:     uninstallCmdName,
		headline: "removed the sshakku hook:",
		apply:    install.Uninstall,
	}, args)
}

// wiring is what differs between the two commands, which is their name, what
// they say they did, and the one function that does it. Everything else — the
// selectors, the refusals, and the shape of the report — is deliberately the
// same, because somebody who has just installed with a set of flags uninstalls
// with them.
type wiring struct {
	name     string
	headline string
	apply    func(context.Context, install.Request, install.Ancestry) (install.Outcome, error)
}

// wire runs one of the two commands and returns the process exit code.
//
// Everything that can refuse is asked before the work is handed over, and a
// refusal is separated from a failure by the exit code: an argument that cannot
// be understood is a usage error, while a shell that cannot be wired is a
// command that ran and could not do it.
func (d deps) wire(ctx context.Context, stdout, stderr io.Writer, command wiring, args []string) int {
	request, err := parseWiringArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s: %v\n", command.name, err)
		return 2
	}

	// What the hook will run is this binary, found where it actually is rather
	// than where an install is meant to put it: a copy run from anywhere wires
	// the copy that was run.
	self, err := d.self()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s: finding this program's own path: %v\n", command.name, err)
		return 1
	}
	request.Binary = self

	outcome, err := command.apply(ctx, request, install.Ancestry{
		Source: newAncestrySource(),
		Image:  install.ImagePath,
		PID:    os.Getpid(),
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s: %v\n", command.name, err)
		return 1
	}

	reportWiring(stdout, command, request, outcome)
	return 0
}

// The three ways an install's arguments can be wrong. The flag keeps the front
// of the sentence in the last two, because what the reader has to correct is
// the flag they typed, not the rule it broke.
var (
	errUnknownArgument = errors.New("unknown argument")
	errTakesNoValue    = errors.New("takes no value")
	errRequiresValue   = errors.New("requires a value")
)

// parseWiringArgs reads the selectors both commands share.
//
// The defaults are the ordinary case: work the shell out from the session this
// was typed in, for the account that typed it, and for every host of a
// PowerShell rather than the one window.
func parseWiringArgs(args []string) (install.Request, error) {
	request := install.Request{Shell: install.Auto, Scope: install.User, Hosts: install.AllHosts}

	for i := 0; i < len(args); i++ {
		name, inline, joined := strings.Cut(args[i], "=")
		takesValue, known := installFlags[name]
		if !known {
			return install.Request{}, fmt.Errorf("%w %q", errUnknownArgument, args[i])
		}
		if !takesValue {
			if joined {
				return install.Request{}, fmt.Errorf("%s %w", name, errTakesNoValue)
			}
			// The only one of them, which is why there is nothing to switch on.
			request.NoPath = true
			continue
		}

		value := inline
		if !joined {
			i++
			if i >= len(args) {
				return install.Request{}, fmt.Errorf("%s %w", name, errRequiresValue)
			}
			value = args[i]
		}

		var err error
		switch name {
		case "--shell":
			request.Shell, err = install.ParseShellKind(value)
		case "--shell-exe":
			request.ShellExe = value
		case "--profile":
			request.Profile = value
		case "--scope":
			request.Scope, err = install.ParseScope(value)
		case "--hosts":
			request.Hosts, err = install.ParseHosts(value)
		}
		if err != nil {
			return install.Request{}, err
		}
	}
	return request, nil
}

// reportWiring prints what was done as the things a person can go and look at:
// the program that was wired, the file that was written, and the hook that file
// now runs. Nothing here is worked out a second time — every line is what the
// step that did the thing reported having done.
func reportWiring(w io.Writer, command wiring, request install.Request, outcome install.Outcome) {
	_, _ = fmt.Fprintln(w, command.headline)

	field := func(name, value string) {
		if value != "" {
			_, _ = fmt.Fprintf(w, "  %-12s %s\n", name, value)
		}
	}
	field("shell", string(outcome.Shell))
	field("interpreter", outcome.Interpreter)
	field("into", outcome.Wired)
	if outcome.Wired != "" {
		if outcome.DropIn {
			field("wrote", "a file of its own, in a drop-in directory the shell reads")
		} else {
			field("wrote", "a marked block, inside the shell's own startup file")
		}
	}
	field("hook", outcome.HookFile)
	field("path", pathStep(command, request, outcome))

	// Not failures, and not something to bury in a log: a drop-in directory
	// that was there and could not be used is why the file named above is the
	// one it is.
	for _, note := range outcome.Notes {
		_, _ = fmt.Fprintf(w, "note: %s\n", note)
	}
}

// pathStep says what became of the step that makes this program runnable by
// name. Systems differ in whether there is anything to record — where the
// program already sits somewhere every session searches, there is not — so what
// is reported is what the step itself said it did.
func pathStep(command wiring, request install.Request, outcome install.Outcome) string {
	switch {
	case request.NoPath:
		return "not recorded (--no-path)"
	case outcome.PathChanged && command.name == installCmdName:
		return "recorded " + outcome.PathEntry
	case outcome.PathChanged:
		return "no longer recorded: " + outcome.PathEntry
	default:
		// Why nothing happened is the system's own answer and is asked for as
		// one: "already there" and "there is nowhere to put it" are both a step
		// that changed nothing, and only one of them is worth going to look at.
		return install.PathStepNothingToDo + " (" + outcome.PathEntry + ")"
	}
}
