package main

import (
	"os"
	"os/user"
	"strings"
)

// userCurrent looks up the invoking process's OS user, a seam so currentUser's
// fallback branch is testable without depending on the process owner being
// resolvable.
var userCurrent = user.Current

// currentUser returns the login name for the secret-store "username" attribute,
// matching $USER so entries the earlier shell version stored are still found.
func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := userCurrent(); err == nil {
		return u.Username
	}
	return ""
}

// shellSingleQuote wraps s in single quotes safe for POSIX shell eval, so paths
// containing spaces or metacharacters survive intact.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
