package install

import "context"

// spelling moves a path between this program's way of writing one and a
// shell's.
//
// Where the two agree there is nothing to do and every method hands the path
// straight back. Where they do not, one translator does all of it, in one
// place: a path translated in some places and not others is the failure this
// exists to prevent, and it is invisible — the file is created, under a name
// the shell never opens.
type spelling struct {
	translator Cygpath
	differs    bool
}

// spellingFor returns how paths must be written for the shell at interpreter.
//
// A translator beside the interpreter is what says the two spellings differ:
// an environment that emulates POSIX on a system that does not ships one, and
// an environment whose paths are already the system's has no use for one.
func spellingFor(interpreter string) spelling {
	if translator, found := FindCygpath(interpreter); found {
		return spelling{translator: translator, differs: true}
	}
	return spelling{}
}

// forShell renders a path the way the shell writes one, which is how it must
// appear inside anything the shell will read.
func (s spelling) forShell(ctx context.Context, path string) (string, error) {
	if !s.differs {
		return path, nil
	}
	return s.translator.ToUnix(ctx, path)
}

// forUs renders a path the way this program writes one, which is how it must
// appear before anything here opens it.
func (s spelling) forUs(ctx context.Context, path string) (string, error) {
	if !s.differs {
		return path, nil
	}
	return s.translator.ToWindows(ctx, path)
}
