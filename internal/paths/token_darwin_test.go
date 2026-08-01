//go:build darwin

package paths

import "testing"

// TestSocketTokenNoKeyring covers the off-Linux token stubs: with no kernel
// keyring there is no per-login socket token, so both accessors yield an empty
// string and the caller degrades to a tokenless socket path.
func TestSocketTokenNoKeyring(t *testing.T) {
	if tok := SocketToken(); tok != "" {
		t.Errorf("SocketToken() = %q, want empty", tok)
	}
	if tok := ReadSocketToken(); tok != "" {
		t.Errorf("ReadSocketToken() = %q, want empty", tok)
	}
}
