package main

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
	shellPosix      = "posix"
	shellPowerShell = "powershell"
)

// shellDialect prints assignments in one shell's language. quote turns a value
// into a literal that language reads back unchanged; varLine and envLine are
// the assignment forms, taking the name and the already-quoted value.
type shellDialect struct {
	name    string
	quote   func(string) string
	varLine string
	envLine string
}

// setVar prints an assignment to a shell variable, newline included.
func (s shellDialect) setVar(name, value string) string {
	return fmt.Sprintf(s.varLine, name, s.quote(value))
}

// setEnv prints an assignment to an environment variable, newline included:
// what a child process this shell starts will inherit.
func (s shellDialect) setEnv(name, value string) string {
	return fmt.Sprintf(s.envLine, name, s.quote(value))
}

// shellDialects is every language this program can print, in the order the
// refusal message offers them. Every value goes through the dialect's own
// quoting, including ones that would need none in a Bourne shell: PowerShell
// reads a bare word after `=` as a command to run, so there is nothing to be
// gained from deciding per value what may be left unquoted.
var shellDialects = []shellDialect{
	{name: shellPosix, quote: shellSingleQuote, varLine: "%s=%s\n", envLine: "export %s=%s\n"},
	{name: shellPowerShell, quote: powerShellSingleQuote, varLine: "$%s = %s\n", envLine: "$env:%s = %s\n"},
}

// shellDialectNamed returns the dialect called name, or an error naming what
// was asked for and what there is instead. A name this program cannot print is
// never quietly answered in some other language: lines a shell cannot read are
// worse than no lines at all, since the shell reports them as its own error.
func shellDialectNamed(name string) (shellDialect, error) {
	for _, d := range shellDialects {
		if d.name == name {
			return d, nil
		}
	}
	names := make([]string, 0, len(shellDialects))
	for _, d := range shellDialects {
		names = append(names, d.name)
	}
	return shellDialect{}, fmt.Errorf("%q is not a shell this program prints for (want %s)",
		name, strings.Join(names, ", "))
}

// dialectFromArgs reads `--shell <name>` or `--shell=<name>` out of a command's
// arguments and returns the dialect to print in. No flag means posix, so the
// shells that had no reason to say which they were keep working unchanged.
// Anything else is a usage error rather than a value to guess at.
func dialectFromArgs(args []string) (shellDialect, error) {
	name := shellPosix
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--shell="):
			name = strings.TrimPrefix(args[i], "--shell=")
		case args[i] == "--shell":
			i++
			if i >= len(args) {
				return shellDialect{}, errors.New("--shell requires a value")
			}
			name = args[i]
		default:
			return shellDialect{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return shellDialectNamed(name)
}

// shellSingleQuote wraps s in single quotes safe for POSIX shell eval, so paths
// containing spaces or metacharacters survive intact. A shell has no escape
// inside such a literal, so an apostrophe in the value is written by leaving
// the literal, quoting the apostrophe on its own, and opening another.
func shellSingleQuote(s string) string {
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
