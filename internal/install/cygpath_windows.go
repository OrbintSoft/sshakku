//go:build windows

package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Cygpath is the path translator belonging to one POSIX-emulating environment.
//
// A shell in such an environment does not see the paths this program writes.
// It has a root of its own — `C:\Program Files\Git` is `/`, a drive is `/c` —
// and a hook wired with a path in the other spelling is a hook that names a
// file the shell cannot open. Which spelling is which is that environment's own
// business, and it ships the answer as a program, so the program is asked
// rather than the mapping guessed at.
type Cygpath struct {
	// Exe is the translator this environment ships.
	Exe string
}

// FindCygpath returns the translator belonging to the environment interpreter
// runs in, or false when there is none to find.
//
// It is found from the interpreter rather than from a fixed installation path,
// because there is no single place these environments live: Git for Windows,
// MSYS2 and Cygwin each have their own root, more than one can be installed at
// once, and a person may have put any of them anywhere. The interpreter being
// wired is the one thing already known, and the translator is its neighbour.
func FindCygpath(interpreter string) (Cygpath, bool) {
	for _, candidate := range cygpathCandidates(interpreter) {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return Cygpath{Exe: candidate}, true
	}
	return Cygpath{}, false
}

// ToUnix renders a path the way the shell in this environment spells it, which
// is the spelling a hook has to carry.
func (c Cygpath) ToUnix(ctx context.Context, path string) (string, error) {
	return c.translate(ctx, "-u", path)
}

// ToWindows renders a path the way this program spells it, for a path a shell
// reported — the home directory it will look for a profile in, for instance.
func (c Cygpath) ToWindows(ctx context.Context, path string) (string, error) {
	return c.translate(ctx, "-w", path)
}

// translate runs the translator and returns the single path it prints.
//
// The translation is lexical: a path that does not exist is translated as
// readily as one that does, which is what lets an install name a file it is
// about to create.
//
// The path is handed over on standard input, and must not be passed as an
// argument. A program built for this environment re-parses the command line it
// was given under that environment's own quoting rules, where an apostrophe
// opens a quoted string — so a path through an account named O'Brien comes back
// with the apostrophe silently gone, naming a directory that does not exist.
// Measured, not feared. Standard input is read as bytes, one path to a line,
// and a path cannot contain a line ending on the system this applies to.
func (c Cygpath) translate(ctx context.Context, how, path string) (string, error) {
	if c.Exe == "" {
		return "", errors.New("no path translator was found for this environment")
	}

	cmd := exec.CommandContext(ctx, c.Exe, how, "-f", "-")
	cmd.Stdin = strings.NewReader(path + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("translating %q with %s: %w%s", path, c.Exe, err, explain(stderr.String()))
	}
	return parseTranslation(stdout, stderr.String(), path, c.Exe)
}

// parseTranslation reads the one path a translator prints.
//
// It is separate from running one so that what is made of an answer can be
// checked on a machine that has no translator to give it — which is every
// machine this program also runs on.
func parseTranslation(stdout []byte, stderr, path, exe string) (string, error) {
	// One path on one line. The trailing newline is the printing, not the path,
	// and a carriage return may be there too depending on how the environment
	// was built; either left in place would be carried into a hook and make the
	// file name unopenable in a way nothing would explain.
	translated := strings.Trim(string(stdout), "\r\n")
	if translated == "" {
		return "", fmt.Errorf("translating %q with %s: it printed nothing%s", path, exe, explain(stderr))
	}
	return translated, nil
}

// cygpathCandidates says where a POSIX-emulating environment keeps its path
// translator, relative to an interpreter belonging to that same environment,
// in the order they should be tried.
//
// Git for Windows is what this is for. It ships a second copy of the shell in
// its own `bin` beside the one in `usr\bin`, and only `usr\bin` holds the
// translator, so from `bin` it is one level up and back down.
//
// The other place is simply beside the interpreter, which is where an
// environment keeping its shell and its translator in one directory puts it.
// That is MSYS2, should it become a target, and Cygwin, which is not one.
// Neither is chased here; the second candidate costs one Stat and means a
// layout this program never had in mind is not excluded by accident.
//
// Both are tried for any interpreter rather than working out which environment
// this is: what makes a candidate the right one is that it is there.
func cygpathCandidates(interpreter string) []string {
	dir := filepath.Dir(interpreter)
	return []string{
		filepath.Join(dir, "cygpath.exe"),
		filepath.Join(filepath.Dir(dir), "usr", "bin", "cygpath.exe"),
	}
}
