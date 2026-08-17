package install

import (
	"context"
	"fmt"
	"os/exec"
)

// lookInterpreter finds an interpreter of the given kind on this system, for
// somebody who named a shell but not a program.
//
// Where a candidate is found the name is not taken as the answer: it goes
// through RecogniseShell, which is the same judgement an ancestry is put
// through. That matters for one name in particular — a system on which two
// unrelated programs are called bash means that the first one on PATH is not
// necessarily a shell an install has any business wiring, and a wiring written
// into the wrong one goes to a filesystem no session of this machine reads.
//
// Finding none is reported with what was looked at, because the remedy is to
// name the program with --shell-exe and that is easier when you can see which
// places were already tried.
func lookInterpreter(ctx context.Context, kind ShellKind) (string, error) {
	tried := interpreterCandidates(ctx, kind)
	for _, candidate := range tried {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		if found, ok := RecogniseShell(path); ok && found == kind {
			return path, nil
		}
	}
	if len(tried) == 0 {
		return "", fmt.Errorf("this system has no %s an install can wire", kind)
	}
	return "", fmt.Errorf("no %s was found: none of %v is one; name the program with --shell-exe", kind, tried)
}

// namedInPatterns is every name this system's table gives to one kind, to be
// looked up on PATH. It is shared by both platforms' candidate lists: a shell
// that is on PATH under its own name needs nothing said about it beyond the
// name the table already holds.
func namedInPatterns(kind ShellKind) []string {
	var names []string
	for _, pattern := range shellPatterns() {
		if pattern.kind == kind {
			names = append(names, pattern.base)
		}
	}
	return names
}
