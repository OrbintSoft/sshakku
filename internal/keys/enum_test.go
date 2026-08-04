package keys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnumeratorKeys(t *testing.T) {
	dir := t.TempDir()
	// Regular key files we expect to find.
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		writeFile(t, filepath.Join(dir, name))
	}
	// Files we must skip: public keys, unrelated files, and a non-id_ name.
	for _, name := range []string{"id_ed25519.pub", "id_rsa.pub", "config", "known_hosts", "authorized_keys"} {
		writeFile(t, filepath.Join(dir, name))
	}
	// A subdirectory named like a key must be skipped (not a regular file).
	if err := os.Mkdir(filepath.Join(dir, "id_dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A symlink named like a key must be skipped (matches `find -type f`).
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(dir, "id_rsa"), filepath.Join(dir, "id_link")); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Enumerator{Dir: dir}.Keys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{filepath.Join(dir, "id_ed25519"), filepath.Join(dir, "id_rsa")}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys = %v, want %v", got, want)
		}
	}
}

func TestEnumeratorMissingDir(t *testing.T) {
	got, err := Enumerator{Dir: filepath.Join(t.TempDir(), "no-such-dir")}.Keys()
	if err != nil {
		t.Fatalf("missing dir should be no error, got %v", err)
	}
	if got != nil {
		t.Fatalf("keys = %v, want nil for a missing dir", got)
	}
}

// TestDefaultKeyPatternsIsTheRuleAndCannotBeChanged covers the naming rule a
// caller has to state rather than apply — F34's report shows what is in force,
// and "nothing" is not what is in force when no patterns are configured. It is
// handed out by value: a caller that edits what it was given must not change
// what the next one is told, or the report and the enumerator come to disagree
// about a rule neither of them was configured with.
func TestDefaultKeyPatternsIsTheRuleAndCannotBeChanged(t *testing.T) {
	got := DefaultKeyPatterns()
	if len(got) == 0 {
		t.Fatal("DefaultKeyPatterns() = empty, want the rule that applies when nobody names one")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "id_rsa"))
	keys, err := Enumerator{Dir: dir, Patterns: got}.Keys()
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys = %v (%v), want the rule to match what the enumerator matches with no patterns at all", keys, err)
	}

	got[0] = "changed"
	if again := DefaultKeyPatterns(); again[0] == "changed" {
		t.Errorf("DefaultKeyPatterns() = %v after a caller edited what it was given, want the rule unchanged", again)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
