//go:build unix

package keys

import (
	"os/exec"
	"testing"
)

// TestBoundToProcessGroupCancelBeforeStart drives the deadline against a command
// that was never started, the race the nil check guards: the context can expire
// between building the Cmd and exec forking it, and Cancel then runs with no
// process behind it.
//
// Without the guard, reading the pid off the nil Process panics, and it panics
// inside the goroutine os/exec runs Cancel on — nothing there recovers, so it
// takes the whole program down rather than failing one command. That is the
// difference the guard makes: a command SSHakku could not start becomes a
// command that simply did not run.
func TestBoundToProcessGroupCancelBeforeStart(t *testing.T) {
	cmd := exec.Command("true")
	boundToProcessGroup(cmd)

	if cmd.Cancel == nil {
		t.Fatal("boundToProcessGroup must install a Cancel for the deadline to call")
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("Cancel() on an unstarted command = %v, want nil", err)
	}
}
