//go:build linux

package main

import (
	"os"
	"strings"
	"testing"
)

// TestExecTokenSourceRequiresRoot exercises only the non-root guard: the actual
// privilege-drop path needs a real root process and another real uid to exec
// as, so it is verified manually / in a multi-user container instead, not
// here.
func TestExecTokenSourceRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the requires-root guard cannot be exercised here")
	}
	_, err := execTokenSource{}.ReadToken(1, 1)
	if err == nil {
		t.Fatal("ReadToken() as non-root = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("ReadToken() error = %q, want it to mention root privileges", err)
	}
}
