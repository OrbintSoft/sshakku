//go:build linux

package keyring

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errEKEYEXPIRED = errors.New("EKEYEXPIRED")
	errENOKEY      = errors.New("ENOKEY")
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
	require.NoError(t, err, "Read")
	assert.Equal(t, payload, got, "Read must return what Add stored")

	found, ok := Search(desc)
	assert.True(t, ok, "Search must find the key just added")
	assert.Equal(t, s, found, "Search must return the serial Add returned")

	require.NoError(t, SetTimeout(s, time.Minute), "SetTimeout")

	require.NoError(t, Unlink(s), "Unlink")
	_, ok = Search(desc)
	assert.False(t, ok, "the key must be gone after Unlink")
}

// TestAddRefusesAnEmptyPayload pins the kernel behaviour the passphrase handoff
// rests on: a "user" key must carry something, so a key stored with nothing in
// it is refused rather than created empty. It runs against the real syscall,
// since a seam could only report back whatever this test told it to say.
func TestAddRefusesAnEmptyPayload(t *testing.T) {
	if !Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	desc := fmt.Sprintf("sshakku-keyring-empty-%d", time.Now().UnixNano())
	s, err := Add(desc, []byte(""))
	if err == nil {
		_ = Unlink(s)
	}
	require.Errorf(t, err, "Add(%q, empty) returned serial %d instead of refusing: what callers do about an empty secret depends on this", desc, s)
}

func TestAddSeam(t *testing.T) {
	t.Run("returns the serial on success", func(t *testing.T) {
		saveKeyctlSeams(t)
		addKey = func(string, string, []byte, int) (int, error) { return 42, nil }
		s, err := Add("desc", []byte("payload"))
		assert.NoError(t, err, "Add")
		assert.Equal(t, Serial(42), s, "Add serial")
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		addKey = func(string, string, []byte, int) (int, error) { return 0, errEDQUOT }
		s, err := Add("desc", nil)
		assert.Error(t, err, "Add must propagate the syscall error")
		assert.Zero(t, s, "Add serial")
	})
}

func TestSearchSeam(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlSearch = func(int, string, string, int) (int, error) { return 7, nil }
		s, ok := Search("desc")
		assert.True(t, ok, "Search must report a hit")
		assert.Equal(t, Serial(7), s, "Search serial")
	})

	t.Run("a missing key is ok=false, not an error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlSearch = func(int, string, string, int) (int, error) { return 0, errENOKEY }
		s, ok := Search("desc")
		assert.False(t, ok, "Search must report a miss")
		assert.Zero(t, s, "Search serial")
	})
}

func TestReadSeam(t *testing.T) {
	t.Run("sizing call fails", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(int, int, []byte, int) (int, error) { return 0, errENOKEY }
		got, err := Read(1)
		assert.Error(t, err, "Read must propagate the sizing failure")
		assert.Nil(t, got, "Read payload")
	})

	t.Run("empty payload returns nil", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(int, int, []byte, int) (int, error) { return 0, nil }
		got, err := Read(1)
		assert.NoError(t, err, "Read")
		assert.Nil(t, got, "an empty key reads as no payload")
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
		assert.NoError(t, err, "Read")
		assert.Equal(t, payload, got, "Read payload")
	})

	t.Run("second read fails", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlBuffer = func(_ int, _ int, buf []byte, _ int) (int, error) {
			if buf == nil {
				return 5, nil
			}
			return 0, errEKEYEXPIRED
		}
		got, err := Read(1)
		assert.Error(t, err, "Read must propagate the second failure")
		assert.Nil(t, got, "Read payload")
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
		assert.NoError(t, err, "Read")
		assert.Equal(t, payload, got, "Read payload")
	})
}

func TestSetTimeoutSeam(t *testing.T) {
	t.Run("rounds a sub-second duration up to one second", func(t *testing.T) {
		saveKeyctlSeams(t)
		var gotSecs int
		keyctlInt = func(_ int, _ int, arg3 int, _ int, _ int) (int, error) { gotSecs = arg3; return 0, nil }
		require.NoError(t, SetTimeout(1, 0), "SetTimeout")
		assert.Equal(t, 1, gotSecs, "seconds passed, rounded up")
	})

	t.Run("passes whole seconds through", func(t *testing.T) {
		saveKeyctlSeams(t)
		var gotSecs int
		keyctlInt = func(_ int, _ int, arg3 int, _ int, _ int) (int, error) { gotSecs = arg3; return 0, nil }
		require.NoError(t, SetTimeout(1, 3*time.Second), "SetTimeout")
		assert.Equal(t, 3, gotSecs, "seconds passed")
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, errENOKEY }
		assert.Error(t, SetTimeout(1, time.Minute), "SetTimeout must propagate the syscall error")
	})
}

func TestUnlinkSeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, nil }
		assert.NoError(t, Unlink(1), "Unlink")
	})

	t.Run("propagates the syscall error", func(t *testing.T) {
		saveKeyctlSeams(t)
		keyctlInt = func(int, int, int, int, int) (int, error) { return 0, errENOKEY }
		assert.Error(t, Unlink(1), "Unlink must propagate the syscall error")
	})
}
