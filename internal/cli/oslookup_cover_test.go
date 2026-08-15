package cli

import (
	"errors"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCurrentUserLookupFailure covers currentUser's last fallback: with $USER
// unset and the OS user lookup failing, it returns the empty string. userCurrent
// is stubbed so the failure is simulated rather than requiring an unresolvable
// process owner.
func TestCurrentUserLookupFailure(t *testing.T) {
	t.Setenv("USER", "")
	orig := userCurrent
	t.Cleanup(func() { userCurrent = orig })
	userCurrent = func() (*user.User, error) { return nil, errors.New("no such user") }

	assert.Empty(t, currentUser(), "with no $USER and no lookup to fall back on, there is no name to give")
}

// TestLookupUserParseFailures covers lookupUser's uid/gid parse-failure branches
// by stubbing the OS user lookup to return an entry whose Uid or Gid is not
// numeric — something the real database never produces, so the guard is only
// reachable through the seam.
func TestLookupUserParseFailures(t *testing.T) {
	origID, origName := userLookupID, userLookup
	t.Cleanup(func() { userLookupID, userLookup = origID, origName })

	t.Run("non-numeric uid", func(t *testing.T) {
		userLookupID = func(string) (*user.User, error) {
			return &user.User{Uid: "not-a-number", Gid: "0", Username: "x"}, nil
		}
		_, err := lookupUser("123")
		assert.Error(t, err, "a uid that is not a number cannot be acted on, and must not be taken for zero")
	})

	t.Run("non-numeric gid", func(t *testing.T) {
		userLookup = func(string) (*user.User, error) {
			return &user.User{Uid: "0", Gid: "not-a-number", Username: "x"}, nil
		}
		_, err := lookupUser("alice")
		assert.Error(t, err, "a gid that is not a number cannot be acted on either")
	})
}
