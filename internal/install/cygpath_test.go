package install

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
