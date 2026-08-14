package config

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "keys/ssh", s.KeyDir, "KeyDir must be the file's own value")
	})

	t.Run("the patterns are kept as written, in order", func(t *testing.T) {
		s, errs := Resolve(File{KeyPatterns: []string{"id_*", "work-*"}}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, []string{"id_*", "work-*"}, s.KeyPatterns, "KeyPatterns must be the file's own list, in order")
	})

	t.Run("naming neither leaves both unset", func(t *testing.T) {
		s, errs := Resolve(File{}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Empty(t, s.KeyDir, "KeyDir must be left unset")
		assert.Nil(t, s.KeyPatterns, "KeyPatterns must be left unset")
	})

	// A pattern that cannot match a file name is worse than a pattern that
	// matches nothing today: the user believes they have widened the rule, and
	// the evidence either way is a key silently absent from the agent.
	t.Run("a pattern that cannot match a file name is refused", func(t *testing.T) {
		for _, bad := range []string{"[unclosed", "keys/id_*", ""} {
			assertPatternsRefused(t, []string{"id_*", bad})
		}
	})

	// Why a pattern was refused is not the same question as whether it was, and
	// the two refusals here are different faults: a glob the shell grammar
	// rejects, and a pattern that is well formed but names a path. Only the
	// wrapped cause tells them apart without re-reading the sentence.
	t.Run("a malformed pattern keeps filepath's own verdict", func(t *testing.T) {
		_, errs := Resolve(File{KeyPatterns: []string{"[unclosed"}}, lookupFrom(nil))
		require.NotEmpty(t, errs, "a malformed pattern must be refused")
		assert.ErrorIs(t, errors.Join(errs...), filepath.ErrBadPattern,
			"the cause has to survive being reported, or nothing downstream can act on it")

		_, errs = Resolve(File{KeyPatterns: []string{"keys/id_*"}}, lookupFrom(nil))
		require.NotEmpty(t, errs, "a pattern holding a separator must be refused")
		assert.NotErrorIs(t, errors.Join(errs...), filepath.ErrBadPattern,
			"and must not be borrowed by the refusal that has nothing to do with the glob grammar")
	})

	// An explicitly empty list is a written instruction, not an omission, and
	// obeying it would leave the user with no keys at all — the very failure
	// this setting exists to remove.
	t.Run("an explicitly empty list is refused", func(t *testing.T) {
		s, errs := Resolve(File{KeyPatterns: []string{}}, lookupFrom(nil))
		assert.Nil(t, s.KeyPatterns, "SSHakku's own rule must be left in force")
		assert.NotEmpty(t, errs, "one error must say the list is empty")
	})
}

// assertPatternsRefused checks that a list holding an unusable pattern is
// refused whole, that SSHakku falls back to its own rule rather than to the
// half of the list it could read, and that the report quotes the offending
// pattern: it is all a user is given to find the line they have to correct.
func assertPatternsRefused(t *testing.T, patterns []string) {
	t.Helper()
	bad := patterns[len(patterns)-1]
	s, errs := Resolve(File{KeyPatterns: patterns}, lookupFrom(nil))
	assert.Nilf(t, s.KeyPatterns, "for %v, SSHakku's own rule must be left in force", patterns)
	var named bool
	for _, err := range errs {
		if strings.Contains(err.Error(), strconv.Quote(bad)) {
			named = true
		}
	}
	assert.Truef(t, named, "errors for %v = %v, want one quoting the rejected pattern", patterns, errs)
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
		assert.Equal(t, filepath.Join(home, ".ssh"), e.Dir, "Dir must be the default under home")
		assert.False(t, e.MustExist, "an absent ~/.ssh must stay an ordinary empty account")
	})

	t.Run("a named directory is resolved against home", func(t *testing.T) {
		for _, written := range []string{"keys/ssh", "~/keys/ssh"} {
			e := Settings{KeyDir: written}.KeyEnumerator(home)
			assert.Equalf(t, filepath.Join(home, "keys", "ssh"), e.Dir, "Dir for %q must be under home", written)
		}
	})

	// What makes a path absolute is the system's answer, not a leading
	// separator — see absRoot.
	t.Run("an absolute directory is taken as it is", func(t *testing.T) {
		absolute := filepath.Join(absRoot, "srv", "keys")
		e := Settings{KeyDir: absolute}.KeyEnumerator(home)
		assert.Equal(t, absolute, e.Dir, "an absolute path must be unchanged")
	})

	// A directory nobody asked for can be absent; one the user named cannot,
	// because a typo and an empty directory produce the same silence otherwise.
	t.Run("a named directory must be there", func(t *testing.T) {
		e := Settings{KeyDir: "keys/ssh"}.KeyEnumerator(home)
		assert.True(t, e.MustExist, "a directory the user named must be required")
	})

	t.Run("the patterns are handed to the enumerator", func(t *testing.T) {
		e := Settings{KeyPatterns: []string{"work-*"}}.KeyEnumerator(home)
		assert.Equal(t, []string{"work-*"}, e.Patterns, "Patterns must be the configured list")
	})
}
