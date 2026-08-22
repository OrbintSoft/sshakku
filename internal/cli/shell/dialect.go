package shell

import (
	"errors"
	"fmt"
	"strings"
)

// The shell languages `--shell` accepts. A command that prints lines for a
// shell to run is told which one is asking; nothing infers it, because the
// operating system does not know — a Windows binary invoked from Git Bash
// wants the Bourne form, and the same binary in a PowerShell window does not.
const (
	Posix      = "posix"
	PowerShell = "powershell"
)

// Dialect prints assignments in one shell's language. quote turns a value
// into a literal that language reads back unchanged; varLine and envLine are
// the assignment forms, taking the name and the already-quoted value.
type Dialect struct {
	name    string
	quote   func(string) string
	varLine string
	envLine string
}

// Name is the language this dialect prints, as `--shell` names it. A caller
// asks when the value it is about to print depends on the shell reading it and
// not only on how that shell is written — a path spelled one way for a shell
// of this system's own and another for one emulating POSIX.
func (s Dialect) Name() string { return s.name }

// Quote renders a value as a literal this shell reads back unchanged, quotes
// included. It is what SetVar and SetEnv put on the right of an assignment, for
// a caller that is placing a value somewhere else — into a hook this program
// renders, say, where the assignment is already written and only the value is
// being supplied.
func (s Dialect) Quote(value string) string {
	return s.quote(value)
}

// SetVar prints an assignment to a shell variable, newline included.
func (s Dialect) SetVar(name, value string) string {
	return fmt.Sprintf(s.varLine, name, s.quote(value))
}

// SetEnv prints an assignment to an environment variable, newline included:
// what a child process this shell starts will inherit.
func (s Dialect) SetEnv(name, value string) string {
	return fmt.Sprintf(s.envLine, name, s.quote(value))
}

// dialects is every language this program can print, in the order the
// refusal message offers them. Every value goes through the dialect's own
// quoting, including ones that would need none in a Bourne shell: PowerShell
// reads a bare word after `=` as a command to run, so there is nothing to be
// gained from deciding per value what may be left unquoted.
var dialects = []Dialect{
	{name: Posix, quote: Quote, varLine: "%s=%s\n", envLine: "export %s=%s\n"},
	{name: PowerShell, quote: powerShellSingleQuote, varLine: "$%s = %s\n", envLine: "$env:%s = %s\n"},
}

// Named returns the dialect called name, for a caller that already knows which
// language it needs and has no flags to read — code rendering a file for a
// shell it has just identified, rather than a command being told what to print.
func Named(name string) (Dialect, error) {
	return named(name)
}

// named returns the dialect called name, or an error naming what
// was asked for and what there is instead. A name this program cannot print is
// never quietly answered in some other language: lines a shell cannot read are
// worse than no lines at all, since the shell reports them as its own error.
func named(name string) (Dialect, error) {
	for _, d := range dialects {
		if d.name == name {
			return d, nil
		}
	}
	names := make([]string, 0, len(dialects))
	for _, d := range dialects {
		names = append(names, d.name)
	}
	return Dialect{}, fmt.Errorf("%q is not a shell this program prints for (want %s)",
		name, strings.Join(names, ", "))
}

// FromArgs reads `--shell <name>` or `--shell=<name>` out of a command's
// arguments and returns the dialect to print in. No flag means posix, so the
// shells that had no reason to say which they were keep working unchanged.
// Anything else is a usage error rather than a value to guess at.
func FromArgs(args []string) (Dialect, error) {
	name := Posix
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--shell="):
			name = strings.TrimPrefix(args[i], "--shell=")
		case args[i] == "--shell":
			i++
			if i >= len(args) {
				return Dialect{}, errors.New("--shell requires a value")
			}
			name = args[i]
		default:
			return Dialect{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return named(name)
}

// Quote wraps s in single quotes safe for POSIX shell eval, so paths
// containing spaces or metacharacters survive intact. A shell has no escape
// inside such a literal, so an apostrophe in the value is written by leaving
// the literal, quoting the apostrophe on its own, and opening another.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// powerShellQuoteChars is every character PowerShell reads as a single quote.
// The apostrophe is the one that is written; the four curly quotes are the
// surprise, and they are not decorative — a literal opened with an apostrophe
// ends at any of them just the same, so a home directory belonging to
// `C:\Users\O’Brien` would end the value halfway through and leave the rest to
// be read as code. Each of them doubled inside a literal stands for itself.
const powerShellQuoteChars = "'\u2018\u2019\u201a\u201b"

// powerShellSingleQuote wraps s in a PowerShell single-quoted literal, doubling
// every character that would otherwise end it. Such a literal is verbatim —
// PowerShell expands nothing inside it, so a `$` or a backtick in a path needs
// no further care.
func powerShellSingleQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		if strings.ContainsRune(powerShellQuoteChars, r) {
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}
