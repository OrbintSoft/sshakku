//go:build darwin

package crossuser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecTokenSourceNoKeyring covers the off-Linux Exec: with no
// kernel keyring there is no per-login token to read, so ReadToken always
// yields an empty token and no error, and no privilege transition is attempted.
func TestExecTokenSourceNoKeyring(t *testing.T) {
	tok, err := Exec{}.ReadToken(t.Context(), 1, 1)
	assert.NoError(t, err, "there being no kernel keyring here is not a failure to report")
	assert.Empty(t, tok, "and there is no per-login token to read")
}
