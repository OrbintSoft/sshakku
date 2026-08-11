package keys

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithDeadline(t *testing.T) {
	t.Run("an answer inside the budget is the answer", func(t *testing.T) {
		got, err := withDeadline("the store", time.Minute, func() (string, error) {
			return "hunter2", nil
		})
		require.NoError(t, err, "a call that answered in time must not be reported as having failed")
		assert.Equal(t, "hunter2", got, "and its answer must come back")
	})

	t.Run("a failure inside the budget is that failure, not a timeout", func(t *testing.T) {
		wantErr := errors.New("no such item")
		_, err := withDeadline("the store", time.Minute, func() (string, error) {
			return "", wantErr
		})
		assert.ErrorIs(t, err, wantErr, "the failure the call reported is the one the caller must see")
		assert.NotErrorIs(t, err, ErrTimedOut,
			"a call that answered in time did not time out, and calling it that hides what went wrong")
	})

	t.Run("a call that never answers is given up on, and says what it was", func(t *testing.T) {
		// Released at the end of the test: giving up does not stop the call,
		// and one still running past the suite is a goroutine goleak fails on.
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		start := time.Now()
		_, err := withDeadline("the keychain", 20*time.Millisecond, func() (string, error) {
			<-release
			return "too late", nil
		})
		require.ErrorIs(t, err, ErrTimedOut, "a call that never answered must be given up on")
		// Loose on purpose: what is being judged is that the budget handed in is
		// the one that is waited out, not how punctual the runtime is about it.
		assert.Less(t, time.Since(start), 2*time.Second,
			"and given up on within the budget it was handed, or a caller that asked for 20ms waits as long as "+
				"something else decided")
		// The session log line is all the user gets: it has to name what was
		// waited on and how long, or "timed out" is not actionable.
		assert.Contains(t, err.Error(), "the keychain", "and say what was waited on")
		assert.Contains(t, err.Error(), "20ms", "and for how long: \"timed out\" on its own is not actionable")
	})

	t.Run("an abandoned call can still finish", func(t *testing.T) {
		// Nothing reads the answer any more, and the call must not be left
		// blocked forever trying to hand one over.
		release := make(chan struct{})
		finished := make(chan struct{})
		_, err := withDeadline("the keychain", 20*time.Millisecond, func() (string, error) {
			<-release
			defer close(finished)
			return "too late", nil
		})
		require.ErrorIs(t, err, ErrTimedOut, "a call that never answered must be given up on")

		close(release)
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			require.FailNow(t, "the abandoned call could not finish",
				"it is stuck handing back an answer nobody wants, which is a goroutine held for the life of the process")
		}
	})
}
