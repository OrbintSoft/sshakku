//go:build ignore

// Command openssh-version stands in for an OpenSSH client that answers only
// the one question being asked of it: which release it is.
//
// It is a real program rather than a mock because what is under test is the
// walk out of SSHakku and back — resolving which ssh this session would run,
// starting it, and reading what it says on the stream it says it on. A stand-in
// that had to be injected somewhere would skip exactly that walk.
//
// The release it claims comes from the environment, so one build serves a
// machine on either side of whatever version is being tested for:
//
//	SSHAKKU_TEST_SSH_VERSION   the line to answer `-V` with; required
//
// OpenSSH prints that line on standard error, and so does this. Anything else
// exits non-zero: this opens no connections, and a caller that got here for any
// other reason is asking for something a stand-in must not appear to provide.
package main

import (
	"fmt"
	"os"
)

const versionEnv = "SSHAKKU_TEST_SSH_VERSION"

func main() {
	said := os.Getenv(versionEnv)
	if said == "" {
		fmt.Fprintf(os.Stderr, "%s names the version to answer with, and is unset\n", versionEnv)
		os.Exit(2)
	}
	if len(os.Args) != 2 || os.Args[1] != "-V" {
		fmt.Fprintln(os.Stderr, "this stand-in answers -V and nothing else")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, said)
}
