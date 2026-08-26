package keyring

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errEACCES = errors.New("EACCES")
	errEDQUOT = errors.New("EDQUOT")
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
		probeAdd = func(string, []byte) (Serial, error) { return 0, errEDQUOT }
		assert.False(t, Available(), "a probe add that fails means unavailable")
	})

	t.Run("a failed read means unavailable", func(t *testing.T) {
		saveProbeSeams(t)
		unlinked := false
		probeAdd = func(string, []byte) (Serial, error) { return 1, nil }
		probeRead = func(Serial) ([]byte, error) { return nil, errEACCES }
		probeUnlink = func(Serial) error { unlinked = true; return nil }
		assert.False(t, Available(), "a probe read that fails means unavailable")
		assert.True(t, unlinked, "the probe key must be unlinked")
	})

	t.Run("a full round trip means available", func(t *testing.T) {
		saveProbeSeams(t)
		unlinked := false
		probeAdd = func(string, []byte) (Serial, error) { return 1, nil }
		probeRead = func(Serial) ([]byte, error) { return []byte("probe"), nil }
		probeUnlink = func(Serial) error { unlinked = true; return nil }
		assert.True(t, Available(), "a probe round trip that succeeds means available")
		assert.True(t, unlinked, "the probe key must be unlinked")
	})
}
