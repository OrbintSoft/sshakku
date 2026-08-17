package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reason BourneLoginFile looks before it writes, asked of a real shell
// rather than remembered.
//
// A login shell reads the first of .bash_profile, .bash_login and .profile that
// it finds, and no others. So an install that always wrote .bash_profile would,
// on an account set up with .profile, not add itself to that account's
// configuration but replace it: the user's own login setup would stop running
// entirely, with nothing said, and they would find out at their next login.
//
// This drives the shell twice over the same home directory — once as the
// account was, once as the install would have left it — and asserts both what
// the shell reads and what BourneLoginFile therefore chooses.
func TestARealShellReadsOneLoginFileAndTheChoiceFollowsIt(t *testing.T) {
	bash := findBash(t, mustAbs(t, hookLib))
	home := t.TempDir()
	spelling := homeAsTheShellSpellsIt(t, bash, home)

	write(t, home, ".profile", "echo READ .profile")
	assert.Equal(t, ".profile", whatTheShellRead(t, bash, spelling),
		"an account set up this way is read from .profile")
	chosen, _, err := BourneLoginFile(spelling, func(path string) bool { return present(home, spelling, path) })
	require.NoError(t, err)
	assert.Equal(t, spelling+"/.profile", chosen, "so that is the file to wire")

	// Now as an install that did not look would have left it.
	write(t, home, ".bash_profile", "echo READ .bash_profile")
	assert.Equal(t, ".bash_profile", whatTheShellRead(t, bash, spelling),
		"the moment this file exists the shell reads it instead, and .profile is not read at all")
	chosen, _, err = BourneLoginFile(spelling, func(path string) bool { return present(home, spelling, path) })
	require.NoError(t, err)
	assert.Equal(t, spelling+"/.bash_profile", chosen, "and now that is the file to wire")
}

func write(t *testing.T, home, name, line string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(home, name), []byte(line+"\n"), 0o644))
}

// present answers whether a path in the shell's spelling is there, by looking
// at the directory this program can see.
func present(home, spelling, path string) bool {
	name := strings.TrimPrefix(path, spelling+"/")
	_, err := os.Stat(filepath.Join(home, name))
	return err == nil
}

// homeAsTheShellSpellsIt is the temporary directory in the shell's own
// spelling, which under a POSIX-emulating environment is not this program's. It
// goes through the same spelling the install itself uses, so this test cannot
// pass by translating differently from the code it is about.
func homeAsTheShellSpellsIt(t *testing.T, bash, home string) string {
	t.Helper()

	spelt, err := spellingFor(bash).forShell(t.Context(), home)
	require.NoError(t, err)
	return filepath.ToSlash(spelt)
}

// whatTheShellRead starts a login shell with the given home and returns which
// of the login files it announced.
func whatTheShellRead(t *testing.T, bash, home string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bash, "--login", "-c", "true")
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	for _, name := range loginFiles {
		if strings.Contains(string(out), "READ "+name) {
			return name
		}
	}
	t.Fatalf("the shell announced no login file it had read: %s", out)
	return ""
}
