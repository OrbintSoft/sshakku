//go:build linux

package keyring

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

// saveKeyctlSeams snapshots the syscall seams and restores them when the
// (sub)test ends, so a test can inject failures without a live kernel keyring
// and without leaking into its siblings.
func saveKeyctlSeams(t *testing.T) {
	t.Helper()
	oAdd, oSearch, oBuf, oInt := addKey, keyctlSearch, keyctlBuffer, keyctlInt
	t.Cleanup(func() { addKey, keyctlSearch, keyctlBuffer, keyctlInt = oAdd, oSearch, oBuf, oInt })
}

func TestKeyringRoundTrip(t *testing.T) {
	if !Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	desc := fmt.Sprintf("sshakku-keyring-test-%d", time.Now().UnixNano())
	payload := []byte("a-secret-passphrase")

	s, err := Add(desc, payload)
	if err != nil {
		t.Skipf("user keyring unavailable: %v", err)
	}
	// Best-effort cleanup if an assertion below aborts before the explicit unlink.
	defer func() { _ = Unlink(s) }()

	got, err := Read(s)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Read = %q, want %q", got, payload)
	}

	if s2, ok := Search(desc); !ok || s2 != s {
		t.Fatalf("Search = (%d, %v), want (%d, true)", s2, ok, s)
	}

	if err := SetTimeout(s, time.Minute); err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}

	if err := Unlink(s); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, ok := Search(desc); ok {
		t.Fatal("key still found after Unlink")
	}
}

func TestAddSeam(t *testing.T) {
	t.Run("returns the serial on success", func(t *testing.T) {
		saveKeyctlSeams(t)
		addKey = func(string, string, []byte, int) (int, error) { return 42, nil }
		s, err := Add("desc", []byte("payload"))
		if err != nil || s != 42 {
			t.Errorf("Add = (%d, %v), want (42, nil)", s, err)
		}
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		addKey = func(string, string, []byte, int) (int, error) { return 0, errors.New("EDQUOT") }
		if s, err := Add("desc", nil); err == nil || s != 0 {
			t.Errorf("Add = (%d, %v), want (0, error)", s, err)
		}
	})
}

func TestSearchSeam(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlSearch = func(int, string, string, int) (int, error) { return 7, nil }
		if s, ok := Search("desc"); !ok || s != 7 {
			t.Errorf("Search = (%d, %v), want (7, true)", s, ok)
		}
	})

	t.Run("a missing key is ok=false, not an error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlSearch = func(int, string, string, int) (int, error) { return 0, errors.New("ENOKEY") }
		if s, ok := Search("desc"); ok || s != 0 {
			t.Errorf("Search = (%d, %v), want (0, false)", s, ok)
		}
	})
}

func TestReadSeam(t *testing.T) {
	t.Run("sizing call fails", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(int, int, []byte, int) (int, error) { return 0, errors.New("ENOKEY") }
		if got, err := Read(1); err == nil || got != nil {
			t.Errorf("Read = (%q, %v), want (nil, error)", got, err)
		}
	})

	t.Run("empty payload returns nil", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(int, int, []byte, int) (int, error) { return 0, nil }
		if got, err := Read(1); err != nil || got != nil {
			t.Errorf("Read = (%q, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("round trip fills the buffer", func(t *testing.T) {
		saveKeyctlSeams(t)
		payload := []byte("hello")
		keyctlBuffer = func(_ int, _ int, buf []byte, _ int) (int, error) {
			if buf == nil {
				return len(payload), nil
			}
			return copy(buf, payload), nil
		}
		got, err := Read(1)
		if err != nil || !bytes.Equal(got, payload) {
			t.Errorf("Read = (%q, %v), want (%q, nil)", got, err, payload)
		}
	})

	t.Run("second read fails", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(_ int, _ int, buf []byte, _ int) (int, error) {
			if buf == nil {
				return 5, nil
			}
			return 0, errors.New("EKEYEXPIRED")
		}
		if got, err := Read(1); err == nil || got != nil {
			t.Errorf("Read = (%q, %v), want (nil, error)", got, err)
		}
	})

	t.Run("an oversized returned length is clamped to the buffer", func(t *testing.T) {
		saveKeyctlSeams(t)
		payload := []byte("abcd")
		keyctlBuffer = func(_ int, _ int, buf []byte, _ int) (int, error) {
			if buf == nil {
				return len(payload), nil
			}
			copy(buf, payload)
			return 100, nil // larger than len(buf): the wrapper must clamp
		}
		got, err := Read(1)
		if err != nil || !bytes.Equal(got, payload) {
			t.Errorf("Read = (%q, %v), want (%q, nil)", got, err, payload)
		}
	})
}

func TestSetTimeoutSeam(t *testing.T) {
	t.Run("rounds a sub-second duration up to one second", func(t *testing.T) {
		saveKeyctlSeams(t)
		var gotSecs int
		keyctlInt = func(_ int, _ int, arg3 int, _ int, _ int) (int, error) { gotSecs = arg3; return 0, nil }
		if err := SetTimeout(1, 0); err != nil {
			t.Fatalf("SetTimeout: %v", err)
		}
		if gotSecs != 1 {
			t.Errorf("seconds passed = %d, want 1 (rounded up)", gotSecs)
		}
	})

	t.Run("passes whole seconds through", func(t *testing.T) {
		saveKeyctlSeams(t)
		var gotSecs int
		keyctlInt = func(_ int, _ int, arg3 int, _ int, _ int) (int, error) { gotSecs = arg3; return 0, nil }
		if err := SetTimeout(1, 3*time.Second); err != nil {
			t.Fatalf("SetTimeout: %v", err)
		}
		if gotSecs != 3 {
			t.Errorf("seconds passed = %d, want 3", gotSecs)
		}
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, errors.New("ENOKEY") }
		if err := SetTimeout(1, time.Minute); err == nil {
			t.Error("SetTimeout returned nil, want the syscall error")
		}
	})
}

func TestUnlinkSeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, nil }
		if err := Unlink(1); err != nil {
			t.Errorf("Unlink = %v, want nil", err)
		}
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, errors.New("ENOKEY") }
		if err := Unlink(1); err == nil {
			t.Error("Unlink returned nil, want the syscall error")
		}
	})
}
