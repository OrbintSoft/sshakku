//go:build darwin

package paths

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSocketTokenNoKeyring covers the off-Linux token stubs: with no kernel
// keyring there is no per-login socket token, so both accessors yield an empty
// string and the caller degrades to a tokenless socket path.
func TestSocketTokenNoKeyring(t *testing.T) {
	assert.Empty(t, SocketToken(), "with no kernel keyring there is no token to create")
	assert.Empty(t, ReadSocketToken(), "with no kernel keyring there is no token to read")
}
