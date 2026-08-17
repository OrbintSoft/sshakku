//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitBash locates the interpreter this is all for, the way findBash does: from
// git itself, since where it is installed is not something to assume.
func gitBash(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	if err != nil {
		t.Skip("no git here, so no Git Bash to find a translator beside")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(out)))))
	bash := filepath.Join(root, "bin", "bash.exe")
	if _, err := exec.LookPath(bash); err != nil {
		t.Skipf("no shell at %s", bash)
	}
	return bash
}

// The translator is found by being there, not by being the first name tried:
// where it sits differs between the environments this may meet, and the layout
// is this platform's own knowledge, so it is checked where it is written down.
func TestTheTranslatorIsFoundWhicheverPlaceItIsIn(t *testing.T) {
	interpreter := filepath.Join(t.TempDir(), "bin", "bash.exe")
	places := cygpathCandidates(interpreter)
	require.NotEmpty(t, places)

	for _, place := range places {
		t.Run(place, func(t *testing.T) {
			require.NoError(t, os.MkdirAll(filepath.Dir(place), 0o755))
			require.NoError(t, os.WriteFile(place, []byte("stands in for a translator"), 0o755))
			t.Cleanup(func() { require.NoError(t, os.Remove(place)) })

			found, ok := FindCygpath(interpreter)

			require.True(t, ok)
			assert.Equal(t, place, found.Exe)
		})
	}
}

// A directory named like the translator is not the translator: running one
// would fail, and be reported as a translation gone wrong rather than as this.
func TestADirectoryIsNotATranslator(t *testing.T) {
	interpreter := filepath.Join(t.TempDir(), "bin", "bash.exe")
	for _, place := range cygpathCandidates(interpreter) {
		require.NoError(t, os.MkdirAll(place, 0o755))
	}

	_, ok := FindCygpath(interpreter)

	assert.False(t, ok)
}

// The point of finding it from the interpreter is that no installation path is
// written down anywhere. This checks the real one is found from the real shell.
func TestTheRealTranslatorIsFoundFromTheRealShell(t *testing.T) {
	bash := gitBash(t)

	cyg, ok := FindCygpath(bash)

	require.True(t, ok, "the shell being wired ships a translator, and it is its neighbour")
	assert.FileExists(t, cyg.Exe)
	assert.Equal(t, "cygpath.exe", filepath.Base(cyg.Exe))
}

// A hook wired for this shell carries a path the shell can open, and this is
// the only test that shows the spelling really changes. The path is one that
// does not exist: the translation is lexical, which is what lets an install
// name a file it is about to create.
func TestARealPathIsSpeltTheWayTheShellSpellsIt(t *testing.T) {
	cyg, ok := FindCygpath(gitBash(t))
	require.True(t, ok)
	windows := `C:\Users\example\AppData\Local\Programs\sshakku\sshakku.exe`

	unix, err := cyg.ToUnix(t.Context(), windows)

	require.NoError(t, err)
	assert.Equal(t, "/c/Users/example/AppData/Local/Programs/sshakku/sshakku.exe", unix,
		"a drive becomes a directory under the root, and the separators turn round")
	assert.NotContains(t, unix, `\`, "a hook carrying a backslash names nothing this shell can open")
}

// The install has to go the other way too: a shell reports the home directory
// it will look for a profile in, and a file has to be written there.
func TestAPathTheShellReportedComesBackAsAPathThisProgramCanUse(t *testing.T) {
	cyg, ok := FindCygpath(gitBash(t))
	require.True(t, ok)

	windows, err := cyg.ToWindows(t.Context(), "/c/Program Files/Git")

	require.NoError(t, err)
	assert.Equal(t, `C:\Program Files\Git`, windows,
		"the space is ordinary here, and a translation that lost it would name a different directory")
}

// Whatever spelling a path starts in, going out and back has to arrive at it,
// or an install and its uninstall are talking about different files.
func TestTranslatingBothWaysReturnsWhereItStarted(t *testing.T) {
	cyg, ok := FindCygpath(gitBash(t))
	require.True(t, ok)

	for _, start := range []string{
		`C:\Users\example\bin\sshakku.exe`,
		`C:\Program Files\Git\usr\bin`,
		`C:\Users\O'Brien\.bashrc`,
	} {
		unix, err := cyg.ToUnix(t.Context(), start)
		require.NoError(t, err)
		back, err := cyg.ToWindows(t.Context(), unix)
		require.NoError(t, err)

		assert.Equal(t, start, back, "%s went out as %s", start, unix)
	}
}

// An account named O'Brien is an ordinary account. The apostrophe is a quoting
// character to the environment this translator was built for, and a path handed
// over as a command-line argument comes back with it silently gone — naming a
// directory that does not exist, in a hook that would fail at every login with
// nothing to say why. Handing the path over as input rather than as an argument
// is what this asserts.
func TestAnApostropheInAPathIsNotEatenOnTheWayThrough(t *testing.T) {
	cyg, ok := FindCygpath(gitBash(t))
	require.True(t, ok)

	// Every character here is one this platform allows in a name. The ones it
	// forbids — " * : < > ? | — are mapped by this environment into a private
	// area of Unicode so that a name containing them can be represented at all,
	// which no path arriving from this platform can ever need.
	for _, path := range []string{
		`C:\Users\O'Brien\.bashrc`,
		`C:\Users\O'Brien\Documents\My Keys\id_ed25519`,
		`C:\Program Files\O'Brien & Co\sshakku.exe`,
	} {
		unix, err := cyg.ToUnix(t.Context(), path)

		require.NoError(t, err)
		assert.Equal(t, strings.Count(path, "'"), strings.Count(unix, "'"),
			"every apostrophe in %q has to still be there in %q", path, unix)
		assert.Equal(t, path, mustToWindows(t, cyg, unix), "and the path has to come back whole")
	}
}

func mustToWindows(t *testing.T, cyg Cygpath, unix string) string {
	t.Helper()
	back, err := cyg.ToWindows(t.Context(), unix)
	require.NoError(t, err)
	return back
}

// The real translator's own refusal, rather than one written from memory: this
// is what the message a user sees is assembled from.
func TestARealRefusalIsReportedWithWhatItSaid(t *testing.T) {
	cyg, ok := FindCygpath(gitBash(t))
	require.True(t, ok)

	_, err := cyg.ToUnix(t.Context(), "")

	require.Error(t, err, "a path that is not a path must not come back as one")
	assert.Contains(t, err.Error(), cyg.Exe)
	assert.Contains(t, strings.ToLower(err.Error()), "convert",
		"and the translator's own words reach the reader; got %q", err)
}

// ---------------------------------------------------------------------------
// What this program makes of a translator's answer. The translator itself is
// asked for real above; these ask nothing of the machine, so they hold wherever
// this file is compiled.

// What a translator answers is the translator's business, and it is asked for
// real in cygpath_windows_test.go, on the only system that has one. What is
// checked here is what this program makes of an answer — which has to hold on
// every machine this program runs on, including the ones that can never see a
// translator at all.
func TestTheAnswerIsTakenAsThePath(t *testing.T) {
	got, err := parseTranslation([]byte("/c/Users/example/bin/sshakku.exe\n"), "", `C:\x`, "cygpath")

	require.NoError(t, err)
	assert.Equal(t, "/c/Users/example/bin/sshakku.exe", got)
}

// The line ending is the printing, not the path. Carried into a hook it makes
// the file name unopenable in a way nothing downstream would explain.
func TestTheLineEndingIsNotPartOfThePath(t *testing.T) {
	for _, ending := range []string{"", "\n", "\r\n", "\n\n"} {
		got, err := parseTranslation([]byte("/c/tmp"+ending), "", `C:\tmp`, "cygpath")

		require.NoError(t, err)
		assert.Equal(t, "/c/tmp", got, "however this environment ends a line")
	}
}

// A space in the path is the ordinary case here, not the exotic one: the
// default installation directory on this platform has one.
func TestASpaceInThePathSurvives(t *testing.T) {
	got, err := parseTranslation([]byte("/c/Program Files/Git\n"), "", `C:\Program Files\Git`, "cygpath")

	require.NoError(t, err)
	assert.Equal(t, "/c/Program Files/Git", got)
}

func TestATranslatorThatSaidNothingIsAFailure(t *testing.T) {
	for _, said := range [][]byte{nil, []byte("\n"), []byte("\r\n")} {
		_, err := parseTranslation(said, "", `C:\tmp`, "cygpath")

		require.Error(t, err, "an empty answer is not an empty path: a hook wired with one names the whole filesystem")
		assert.Contains(t, err.Error(), "printed nothing")
		assert.Contains(t, err.Error(), strconv.Quote(`C:\tmp`), "and the message says which path went unanswered")
	}
}

// There can be more than one of these on a machine, belonging to different
// environments, so a failure has to say which one failed.
func TestAFailureNamesTheTranslatorAndRepeatsWhatItSaid(t *testing.T) {
	_, err := parseTranslation(nil, "cygpath: can't convert empty path", "", "/git/usr/bin/cygpath.exe")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "/git/usr/bin/cygpath.exe")
	assert.Contains(t, err.Error(), "can't convert empty path")
}

func TestATranslatorThatCouldNotBeRunIsAFailureThatNamesIt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-a-translator")

	_, err := Cygpath{Exe: missing}.ToUnix(t.Context(), `C:\tmp`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
	assert.Contains(t, err.Error(), strconv.Quote(`C:\tmp`))
}

func TestNoTranslatorAtAllIsSaidRatherThanRun(t *testing.T) {
	_, err := Cygpath{}.ToUnix(t.Context(), `C:\tmp`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no path translator")
}

// True wherever this runs, and for opposite reasons: on a system with no
// translator there is nowhere to look, and on one that has them there is
// nothing at the places looked in.
func TestAnInterpreterWithNoTranslatorNearItIsSaidSo(t *testing.T) {
	_, ok := FindCygpath(filepath.Join(t.TempDir(), "bin", "bash"))

	assert.False(t, ok, "an environment with no translator has none, and must not be handed an empty one")
}
