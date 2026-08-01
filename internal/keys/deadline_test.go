package keys

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWithDeadline(t *testing.T) {
	t.Run("an answer inside the budget is the answer", func(t *testing.T) {
		got, err := withDeadline("the store", time.Minute, func() (string, error) {
			return "hunter2", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hunter2" {
			t.Errorf("value = %q, want hunter2", got)
		}
	})

	t.Run("a failure inside the budget is that failure, not a timeout", func(t *testing.T) {
		wantErr := errors.New("no such item")
		_, err := withDeadline("the store", time.Minute, func() (string, error) {
			return "", wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if errors.Is(err, ErrTimedOut) {
			t.Error("a call that answered was reported as having timed out")
		}
	})

	t.Run("a call that never answers is given up on, and says what it was", func(t *testing.T) {
		// Released at the end of the test: giving up does not stop the call,
		// and one still running past the suite is a goroutine goleak fails on.
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		_, err := withDeadline("the keychain", 20*time.Millisecond, func() (string, error) {
			<-release
			return "too late", nil
		})
		if !errors.Is(err, ErrTimedOut) {
			t.Fatalf("error = %v, want ErrTimedOut", err)
		}
		// The session log line is all the user gets: it has to name what was
		// waited on and how long, or "timed out" is not actionable.
		if !strings.Contains(err.Error(), "the keychain") || !strings.Contains(err.Error(), "20ms") {
			t.Errorf("error = %q, want it to name the keychain and the budget", err)
		}
	})

	t.Run("an abandoned call can still finish", func(t *testing.T) {
		// Nothing reads the answer any more, and the call must not be left
		// blocked forever trying to hand one over.
		release := make(chan struct{})
		finished := make(chan struct{})
		if _, err := withDeadline("the keychain", 20*time.Millisecond, func() (string, error) {
			<-release
			defer close(finished)
			return "too late", nil
		}); !errors.Is(err, ErrTimedOut) {
			t.Fatalf("error = %v, want ErrTimedOut", err)
		}

		close(release)
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Fatal("the abandoned call could not finish: it is stuck handing back an answer nobody wants")
		}
	})
}
