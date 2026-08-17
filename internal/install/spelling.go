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
//
// The translating itself is a system's own knowledge and lives with that
// system; what arrives here is the pair of directions, or nothing at all. That
// keeps this readable — and answerable — from a machine that has no translator
// to hand and never will.
type spelling struct {
	// toShell and toUs render a path the shell's way and this program's way.
	// Both are nil together, on a system where the two spellings agree.
	toShell func(context.Context, string) (string, error)
	toUs    func(context.Context, string) (string, error)
}

// forShell renders a path the way the shell writes one, which is how it must
// appear inside anything the shell will read.
func (s spelling) forShell(ctx context.Context, path string) (string, error) {
	if s.toShell == nil {
		return path, nil
	}
	return s.toShell(ctx, path)
}

// forUs renders a path the way this program writes one, which is how it must
// appear before anything here opens it.
func (s spelling) forUs(ctx context.Context, path string) (string, error) {
	if s.toUs == nil {
		return path, nil
	}
	return s.toUs(ctx, path)
}
