//go:build !darwin

package keys

import (
	"errors"
	"testing"
)

// TestNoKeychainClientUnavailable checks that every NoKeychainClient method
// reports ErrKeychainUnavailable off macOS, where there is no keychain to talk
// to.
func TestNoKeychainClientUnavailable(t *testing.T) {
	var c NoKeychainClient

	if err := c.Add("acct", "svc", "label", "pass"); !errors.Is(err, ErrKeychainUnavailable) {
		t.Errorf("Add = %v, want ErrKeychainUnavailable", err)
	}
	if err := c.Update("acct", "svc", "pass"); !errors.Is(err, ErrKeychainUnavailable) {
		t.Errorf("Update = %v, want ErrKeychainUnavailable", err)
	}
	if pass, found, err := c.Find("acct", "svc"); pass != "" || found || !errors.Is(err, ErrKeychainUnavailable) {
		t.Errorf("Find = (%q, %v, %v), want (\"\", false, ErrKeychainUnavailable)", pass, found, err)
	}
	if err := c.Delete("acct", "svc"); !errors.Is(err, ErrKeychainUnavailable) {
		t.Errorf("Delete = %v, want ErrKeychainUnavailable", err)
	}
	if list, err := c.List("acct"); list != nil || !errors.Is(err, ErrKeychainUnavailable) {
		t.Errorf("List = (%v, %v), want (nil, ErrKeychainUnavailable)", list, err)
	}
}
