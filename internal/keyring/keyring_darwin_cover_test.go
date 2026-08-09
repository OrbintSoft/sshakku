//go:build darwin

package keyring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestOtherStubsUnavailable checks that every keyring operation degrades to
// ErrUnavailable (or "no key") off Linux, where there is no kernel keyring to
// store secrets in.
func TestOtherStubsUnavailable(t *testing.T) {
	addSerial, err := Add("name", []byte("secret"))
	assert.Zero(t, addSerial, "Add serial")
	assert.ErrorIs(t, err, ErrUnavailable, "Add")

	searchSerial, ok := Search("name")
	assert.Zero(t, searchSerial, "Search serial")
	assert.False(t, ok, "Search must not find a key")

	payload, err := Read(1)
	assert.Nil(t, payload, "Read payload")
	assert.ErrorIs(t, err, ErrUnavailable, "Read")

	assert.ErrorIs(t, SetTimeout(1, time.Minute), ErrUnavailable, "SetTimeout")
	assert.ErrorIs(t, Unlink(1), ErrUnavailable, "Unlink")
}
