package keys

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnumeratorReadDirHardError covers Keys's non-missing-directory error
// branch: pointing the enumerator at a regular file (not a directory) makes
// ReadDir fail with something other than "does not exist", which propagates.
func TestEnumeratorReadDirHardError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if keys, err := (Enumerator{Dir: file}).Keys(); err == nil || keys != nil {
		t.Fatalf("Keys = (%v, %v), want (nil, error) reading a non-directory", keys, err)
	}
}

// TestKDialogAvailableDefaultLookPath covers Available's nil-lookPath branch,
// which falls back to the real os/exec PATH lookup. The result depends on
// whether kdialog happens to be installed; only the branch matters here.
func TestKDialogAvailableDefaultLookPath(t *testing.T) {
	_ = KDialogPrompter{}.Available()
}

// TestExecRunnerRunStdinEnvAndStartFailure covers Run's stdin- and env-passing
// branches (via a real `cat` that echoes its stdin) and its start-failure
// branch (a binary that does not exist, which is a real error rather than a
// non-zero exit code).
func TestExecRunnerRunStdinEnvAndStartFailure(t *testing.T) {
	res, err := ExecRunner{}.Run(Cmd{Name: "cat", Stdin: "hello", Env: []string{"SSHAKKU_X=1"}})
	if err != nil {
		t.Fatalf("cat: unexpected error: %v", err)
	}
	if string(res.Stdout) != "hello" {
		t.Fatalf("cat stdout = %q, want %q", res.Stdout, "hello")
	}

	if _, err := (ExecRunner{}).Run(Cmd{Name: "sshakku-no-such-binary-xyz"}); err == nil {
		t.Fatal("running a nonexistent binary returned nil error, want a start failure")
	}
}
