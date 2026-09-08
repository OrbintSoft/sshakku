//go:build windows

package cli

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTheEditorNobodyNamedIsOneThisSystemHas verifies the half of F36 that a
// user meets on an account nobody has set up: where no editor is named
// anywhere, the one SSHakku falls back on has to be a program this system can
// actually run, or `--edit` leaves them with a file and an error instead of an
// editor.
//
// The claim is made for this system and not for every system because it is
// only true here. Notepad is part of Windows, so an account without an editor
// is a promise broken; vi is a package a minimal installation elsewhere may
// simply not have, and a suite that failed there would be reporting the
// machine rather than the product.
func TestTheEditorNobodyNamedIsOneThisSystemHas(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	command := editorCommand(t.TempDir())
	require.NotEmpty(t, command, "some editor has to be run")

	_, err := exec.LookPath(command[0])
	require.NoErrorf(t, err,
		"with no editor named anywhere SSHakku falls back on %q, which this system has no way to run",
		command[0])
}
