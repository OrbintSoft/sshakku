// Command sshakku tends the SSH agent: it computes the per-user runtime
// paths, keeps the agent healthy, and loads keys with passphrases pulled from
// the OS secret store. The login shell wires it in by evaluating its output:
//
//	eval "$(sshakku shell-init)"
package main

import (
	"context"
	"os"

	"github.com/OrbintSoft/sshakku/internal/cli"
)

func main() {
	// The process entry point: a single os.Exit around the testable command,
	// with nothing here to unit-test (a test can't observe os.Exit).
	//
	// The context created here is the program's only root. Everything that
	// waits on something outside this process — the session bus, ssh-add, a
	// wallet's CLI, a socket — derives its own from the one passed down, so a
	// deadline or a cancellation set above a wait reaches the wait itself. A
	// second root anywhere below would be where that stops being true.
	//coverage:ignore
	os.Exit(cli.Main(context.Background(), os.Stdout, os.Stderr, os.Args[0], os.Args[1:]))
}
