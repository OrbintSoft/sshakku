package main

import (
	"errors"
	"os/user"
	"testing"
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

	if got := currentUser(); got != "" {
		t.Errorf("currentUser() = %q, want \"\" when the lookup fails", got)
	}
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
		if _, err := lookupUser("123"); err == nil {
			t.Error("lookupUser = nil error, want a uid parse failure")
		}
	})

	t.Run("non-numeric gid", func(t *testing.T) {
		userLookup = func(string) (*user.User, error) {
			return &user.User{Uid: "0", Gid: "not-a-number", Username: "x"}, nil
		}
		if _, err := lookupUser("alice"); err == nil {
			t.Error("lookupUser = nil error, want a gid parse failure")
		}
	})
}
