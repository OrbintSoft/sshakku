//go:build !linux

package keyring

import (
	"errors"
	"testing"
	"time"
)

// TestOtherStubsUnavailable checks that every keyring operation degrades to
// ErrUnavailable (or "no key") off Linux, where there is no kernel keyring to
// store secrets in.
func TestOtherStubsUnavailable(t *testing.T) {
	if s, err := Add("name", []byte("secret")); s != 0 || !errors.Is(err, ErrUnavailable) {
		t.Errorf("Add = (%d, %v), want (0, ErrUnavailable)", s, err)
	}
	if s, ok := Search("name"); s != 0 || ok {
		t.Errorf("Search = (%d, %v), want (0, false)", s, ok)
	}
	if b, err := Read(1); b != nil || !errors.Is(err, ErrUnavailable) {
		t.Errorf("Read = (%v, %v), want (nil, ErrUnavailable)", b, err)
	}
	if err := SetTimeout(1, time.Minute); !errors.Is(err, ErrUnavailable) {
		t.Errorf("SetTimeout = %v, want ErrUnavailable", err)
	}
	if err := Unlink(1); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Unlink = %v, want ErrUnavailable", err)
	}
}
