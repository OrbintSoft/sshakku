//go:build darwin

package main

import "testing"

// TestExecTokenSourceNoKeyring covers the off-Linux execTokenSource: with no
// kernel keyring there is no per-login token to read, so ReadToken always
// yields an empty token and no error, and no privilege transition is attempted.
func TestExecTokenSourceNoKeyring(t *testing.T) {
	tok, err := execTokenSource{}.ReadToken(1, 1)
	if tok != "" || err != nil {
		t.Errorf("ReadToken = (%q, %v), want (\"\", nil)", tok, err)
	}
}
