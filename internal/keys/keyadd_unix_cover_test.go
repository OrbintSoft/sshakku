//go:build unix

package keys

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

// saveKeyaddSeams snapshots the stash and ssh-add-run seams, restoring them
// when the (sub)test ends.
func saveKeyaddSeams(t *testing.T) {
	t.Helper()
	oStash, oRun := stashPass, runCmd
	t.Cleanup(func() { stashPass, runCmd = oStash, oRun })
}

func TestAddWithAskpassStashError(t *testing.T) {
	saveKeyaddSeams(t)
	stashPass = func(string, time.Duration) (string, error) { return "", errors.New("no keyring") }
	a := ExecKeyAdder{AskpassProg: "/usr/bin/sshakku"}
	if _, err := a.AddWithAskpass("/home/u/.ssh/id_rsa", "pw"); err == nil {
		t.Fatal("AddWithAskpass returned nil error, want the stash failure")
	}
}

func TestAddWithAskpassRunsSSHAdd(t *testing.T) {
	saveKeyaddSeams(t)
	var stashedTTL time.Duration
	stashPass = func(_ string, ttl time.Duration) (string, error) { stashedTTL = ttl; return "token", nil }
	runCmd = func(*exec.Cmd) error { return nil }
	a := ExecKeyAdder{AskpassProg: "/usr/bin/sshakku"}
	rc, err := a.AddWithAskpass("/home/u/.ssh/id_rsa", "pw")
	if err != nil || rc != 0 {
		t.Fatalf("AddWithAskpass = (%d, %v), want (0, nil)", rc, err)
	}
	if stashedTTL != defaultKeyTTL {
		t.Fatalf("stash ttl = %v, want the default %v", stashedTTL, defaultKeyTTL)
	}
}

func TestRunSSHAddExitCode(t *testing.T) {
	saveKeyaddSeams(t)
	// A real non-zero process exit yields the *exec.ExitError runSSHAdd must
	// translate into a returned exit code (a wrong passphrase, not a failure).
	realExit := exec.Command("sh", "-c", "exit 3").Run()
	var ee *exec.ExitError
	if !errors.As(realExit, &ee) {
		t.Skipf("could not obtain an ExitError in this environment: %v", realExit)
	}
	runCmd = func(*exec.Cmd) error { return realExit }

	rc, err := (ExecKeyAdder{}).runSSHAdd(nil, "/home/u/.ssh/id_rsa")
	if err != nil {
		t.Fatalf("runSSHAdd err = %v, want nil for a non-zero exit", err)
	}
	if rc != 3 {
		t.Fatalf("runSSHAdd rc = %d, want 3", rc)
	}
}

func TestRunSSHAddStartFailure(t *testing.T) {
	saveKeyaddSeams(t)
	runCmd = func(*exec.Cmd) error { return errors.New("fork/exec: permission denied") }
	rc, err := (ExecKeyAdder{}).runSSHAdd(nil, "/home/u/.ssh/id_rsa")
	if err == nil || rc != 0 {
		t.Fatalf("runSSHAdd = (%d, %v), want (0, error) on a start failure", rc, err)
	}
}
