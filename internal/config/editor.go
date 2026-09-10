package config

import (
	"strings"
	"unicode"
)

// EditorCommand is the editor `sshakku config --edit` opens a file in: the
// program to run and the arguments it was named with. What the configuration
// or the environment stated is a command line and is cut into one; where
// nothing was stated it is the first of this system's own editors that can be
// found.
func EditorCommand(s Settings) []string {
	if stated := strings.TrimSpace(s.Editor); stated != "" {
		return splitCommandLine(stated)
	}
	// A program found on this system is one program, however many spaces its
	// path has in it: it was never a command line and is not cut into one.
	return []string{firstEditorFound(platformEditors, findEditor)}
}

// EditorInForce names the editor that would be opened, for a report that has
// to say what is in force without opening anything.
func EditorInForce(s Settings) string {
	if stated := strings.TrimSpace(s.Editor); stated != "" {
		return stated
	}
	return firstEditorFound(platformEditors, findEditor)
}

// firstEditorFound picks the first of this system's own editors that can be
// run. Both the editors and the looking are handed in, so each system's answer
// stays checkable from a machine that is not the one it names.
func firstEditorFound(candidates []string, find func(string) (string, bool)) string {
	for _, name := range candidates {
		if path, found := find(name); found {
			return path
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	// None of them is here. Naming the one that would have been opened leaves
	// the reader a program to go and install, where saying nothing would leave
	// them a command that failed without saying what it wanted.
	return candidates[0]
}

// splitCommandLine cuts a command line into the program and the arguments
// after it, on spaces, except where double quotes hold a run of them together.
// An editor is named as a command line ("code -w", "emacs -nw") rather than as
// a bare program name, and honouring only the first word would run some
// editors in a mode their owner never uses.
//
// Quoting is what makes such a name writable at all on a system whose programs
// are installed under paths with spaces in them, and it is the quoting those
// systems already use, so a path can be pasted from where it was found.
func splitCommandLine(line string) []string {
	var fields []string
	var current strings.Builder
	quoted, started := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted, started = !quoted, true
		case !quoted && unicode.IsSpace(r):
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if started {
		fields = append(fields, current.String())
	}
	return fields
}
