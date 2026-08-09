//go:build darwin

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecTokenSourceNoKeyring covers the off-Linux execTokenSource: with no
// kernel keyring there is no per-login token to read, so ReadToken always
// yields an empty token and no error, and no privilege transition is attempted.
func TestExecTokenSourceNoKeyring(t *testing.T) {
	tok, err := execTokenSource{}.ReadToken(1, 1)
	assert.NoError(t, err, "there being no kernel keyring here is not a failure to report")
	assert.Empty(t, tok, "and there is no per-login token to read")
}
