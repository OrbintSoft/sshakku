package config

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestResolveKeyDiscovery covers the two settings that say which files SSHakku
// treats as your keys (F34): the directory to look in and the names to
// recognise there.
//
// Neither resolves to a value when the user names none. The default directory
// is only expressible against a home directory this layer does not know, and
// the default name rule belongs to the enumerator that applies it — the job
// here is to decide whether the user said anything and whether what they said
// can be used at all.
func TestResolveKeyDiscovery(t *testing.T) {
	t.Run("the directory is kept as written", func(t *testing.T) {
		s, errs := Resolve(File{KeyDir: ptr("keys/ssh")}, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if s.KeyDir != "keys/ssh" {
			t.Errorf("KeyDir = %q, want the file's own value", s.KeyDir)
		}
	})

	t.Run("the patterns are kept as written, in order", func(t *testing.T) {
		s, errs := Resolve(File{KeyPatterns: []string{"id_*", "work-*"}}, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if strings.Join(s.KeyPatterns, ",") != "id_*,work-*" {
			t.Errorf("KeyPatterns = %v, want the file's own list", s.KeyPatterns)
		}
	})

	t.Run("naming neither leaves both unset", func(t *testing.T) {
		s, errs := Resolve(File{}, lookupFrom(nil))
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if s.KeyDir != "" || s.KeyPatterns != nil {
			t.Errorf("KeyDir/KeyPatterns = %q/%v, want both left unset", s.KeyDir, s.KeyPatterns)
		}
	})

	// A pattern that cannot match a file name is worse than a pattern that
	// matches nothing today: the user believes they have widened the rule, and
	// the evidence either way is a key silently absent from the agent.
	t.Run("a pattern that cannot match a file name is refused", func(t *testing.T) {
		for _, bad := range []string{"[unclosed", "keys/id_*", ""} {
			assertPatternsRefused(t, []string{"id_*", bad})
		}
	})

	// An explicitly empty list is a written instruction, not an omission, and
	// obeying it would leave the user with no keys at all — the very failure
	// this setting exists to remove.
	t.Run("an explicitly empty list is refused", func(t *testing.T) {
		s, errs := Resolve(File{KeyPatterns: []string{}}, lookupFrom(nil))
		if s.KeyPatterns != nil {
			t.Errorf("KeyPatterns = %v, want SSHakku's own rule left in force", s.KeyPatterns)
		}
		if len(errs) == 0 {
			t.Error("errors = none, want one saying the list is empty")
		}
	})
}

// assertPatternsRefused checks that a list holding an unusable pattern is
// refused whole, that SSHakku falls back to its own rule rather than to the
// half of the list it could read, and that the report quotes the offending
// pattern: a login shell writes this to the session log and nowhere else.
func assertPatternsRefused(t *testing.T, patterns []string) {
	t.Helper()
	bad := patterns[len(patterns)-1]
	s, errs := Resolve(File{KeyPatterns: patterns}, lookupFrom(nil))
	if s.KeyPatterns != nil {
		t.Errorf("KeyPatterns = %v for %v, want SSHakku's own rule left in force", s.KeyPatterns, patterns)
	}
	var named bool
	for _, err := range errs {
		if strings.Contains(err.Error(), strconv.Quote(bad)) {
			named = true
		}
	}
	if !named {
		t.Errorf("errors for %v = %v, want one quoting the rejected pattern", patterns, errs)
	}
}

// TestKeyEnumeratorFromSettings covers the one place the two settings are
// turned into the enumerator every caller uses (F34). It is one place on
// purpose: `load-keys` and `doctor` each built their own directory before, and
// a doctor that reports a different set of keys from the one SSHakku acts on is
// worse than no report at all.
func TestKeyEnumeratorFromSettings(t *testing.T) {
	const home = "/home/u"

	t.Run("naming no directory looks where OpenSSH keeps keys", func(t *testing.T) {
		e := Settings{}.KeyEnumerator(home)
		if e.Dir != filepath.Join(home, ".ssh") {
			t.Errorf("Dir = %q, want the default under home", e.Dir)
		}
		if e.MustExist {
			t.Error("MustExist = true, want an absent ~/.ssh to stay an ordinary empty account")
		}
	})

	t.Run("a named directory is resolved against home", func(t *testing.T) {
		for _, written := range []string{"keys/ssh", "~/keys/ssh"} {
			e := Settings{KeyDir: written}.KeyEnumerator(home)
			if e.Dir != filepath.Join(home, "keys", "ssh") {
				t.Errorf("Dir = %q for %q, want it under home", e.Dir, written)
			}
		}
	})

	t.Run("an absolute directory is taken as it is", func(t *testing.T) {
		e := Settings{KeyDir: "/srv/keys"}.KeyEnumerator(home)
		if e.Dir != "/srv/keys" {
			t.Errorf("Dir = %q, want the absolute path unchanged", e.Dir)
		}
	})

	// A directory nobody asked for can be absent; one the user named cannot,
	// because a typo and an empty directory produce the same silence otherwise.
	t.Run("a named directory must be there", func(t *testing.T) {
		if !(Settings{KeyDir: "keys/ssh"}).KeyEnumerator(home).MustExist {
			t.Error("MustExist = false, want a directory the user named to be required")
		}
	})

	t.Run("the patterns are handed to the enumerator", func(t *testing.T) {
		e := Settings{KeyPatterns: []string{"work-*"}}.KeyEnumerator(home)
		if strings.Join(e.Patterns, ",") != "work-*" {
			t.Errorf("Patterns = %v, want the configured list", e.Patterns)
		}
	})
}
