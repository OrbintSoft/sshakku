//go:build linux

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecTokenSourceRequiresRoot exercises only the non-root guard: the actual
// privilege-drop path needs a real root process and another real uid to exec
// as, so it is verified manually / in a multi-user container instead, not
// here.
func TestExecTokenSourceRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: the requires-root guard cannot be exercised here")
	}
	_, err := execTokenSource{}.ReadToken(t.Context(), 1, 1)
	require.Error(t, err, "reading another user's keyring without root must be refused, not attempted")
	assert.ErrorContains(t, err, "root", "and the refusal must say what would be needed")
}
