package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// binaryBesideItsHelper puts this program and the helper it is asked through in
// one directory, which is what an install leaves behind, and returns the path of
// the program. Nothing runs either of them: what these tests are about is which
// lines a shell is handed, and that is decided by looking.
func binaryBesideItsHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	self := filepath.Join(dir, "sshakku"+programSuffix)
	require.NoError(t, os.WriteFile(self, []byte("a program\n"), 0o700))              //nolint:gosec // G306: a file standing in for a program, which is not one unless it can be run
	require.NoError(t, os.WriteFile(askpassProg(self), []byte("a program\n"), 0o700)) //nolint:gosec // G306: the same, for the helper beside it
	return self
}

// TestAskpassEnvSaysNothingWhereThereIsNoHelperToPointAt.
//
// The two lines are worth printing together or not at all. REQUIRE=force is
// what takes ssh's own terminal prompt away, and pointing it at a program that
// is not there does not degrade the session — it removes the only way that
// session had of being asked anything: OpenSSH answers a helper it cannot start
// by handing ssh an empty passphrase, so a key that needs one simply stops
// opening, with the failure reported against the key. Printing nothing leaves
// the shell exactly as it would be on a machine where SSHakku was never
// installed, which is a session that can still be asked.
func TestAskpassEnvSaysNothingWhereThereIsNoHelperToPointAt(t *testing.T) {
	dir := t.TempDir()
	d := realDeps()
	d.self = func() (string, error) { return filepath.Join(dir, "sshakku"+programSuffix), nil }
	var out, errOut bytes.Buffer

	require.Zero(t, d.askpassEnv(&out, &errOut, nil),
		"a shell that cannot be wired this way still opens, and openly: this is not a failure")
	assert.Empty(t, out.String(),
		"a shell pointed at a helper that is not there can no longer be asked for a passphrase at all")
	assert.Empty(t, errOut.String(),
		"and nothing is said here, since doctor names it and the loader tells the user once")
}

// TestAskpassEnvPrintsTheLinesWhereTheHelperIsThere is the ordinary case, and
// the reason the one above cannot be had by printing nothing always.
func TestAskpassEnvPrintsTheLinesWhereTheHelperIsThere(t *testing.T) {
	d := realDeps()
	self := binaryBesideItsHelper(t)
	d.self = func() (string, error) { return self, nil }
	var out, errOut bytes.Buffer

	require.Zerof(t, d.askpassEnv(&out, &errOut, nil), "askpassEnv; stderr=%q", errOut.String())
	assert.Contains(t, out.String(), "export SSH_ASKPASS='"+askpassProg(self)+"'")
	assert.Contains(t, out.String(), "export SSH_ASKPASS_REQUIRE='force'")
}
