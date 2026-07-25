package keyring

import (
	"errors"
	"testing"
)

// saveProbeSeams snapshots Available's probe seams and restores them when the
// (sub)test ends, so a test can drive its branches without a live keyring.
func saveProbeSeams(t *testing.T) {
	t.Helper()
	oAdd, oUnlink, oRead := probeAdd, probeUnlink, probeRead
	t.Cleanup(func() { probeAdd, probeUnlink, probeRead = oAdd, oUnlink, oRead })
}

func TestAvailableSeam(t *testing.T) {
	t.Run("a failed add means unavailable", func(t *testing.T) {
		saveProbeSeams(t)
		probeAdd = func(string, []byte) (Serial, error) { return 0, errors.New("EDQUOT") }
		if Available() {
			t.Error("Available() = true, want false when the probe add fails")
		}
	})

	t.Run("a failed read means unavailable", func(t *testing.T) {
		saveProbeSeams(t)
		unlinked := false
		probeAdd = func(string, []byte) (Serial, error) { return 1, nil }
		probeRead = func(Serial) ([]byte, error) { return nil, errors.New("EACCES") }
		probeUnlink = func(Serial) error { unlinked = true; return nil }
		if Available() {
			t.Error("Available() = true, want false when the probe read fails")
		}
		if !unlinked {
			t.Error("Available() did not unlink the probe key")
		}
	})

	t.Run("a full round trip means available", func(t *testing.T) {
		saveProbeSeams(t)
		unlinked := false
		probeAdd = func(string, []byte) (Serial, error) { return 1, nil }
		probeRead = func(Serial) ([]byte, error) { return []byte("probe"), nil }
		probeUnlink = func(Serial) error { unlinked = true; return nil }
		if !Available() {
			t.Error("Available() = false, want true when the probe round trip succeeds")
		}
		if !unlinked {
			t.Error("Available() did not unlink the probe key")
		}
	})
}
