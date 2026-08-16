//go:build unix

package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This system is the thing the other one emulates, so a shell and this program
// spell a path the same way and there is nothing to translate between. Saying
// so is an answer, and it is asserted here rather than left to a test that
// quietly declines to run: a translator that turned up on this system would
// mean paths were about to be rewritten that should be passed through whole.
func TestThisSystemKeepsNoTranslatorAndNeedsNone(t *testing.T) {
	assert.Empty(t, cygpathCandidates("/usr/bin/bash"),
		"there is nowhere to look, because there is nothing to look for")

	_, ok := FindCygpath("/usr/bin/bash")
	assert.False(t, ok)

	_, ok = FindCygpath("/opt/homebrew/bin/bash")
	assert.False(t, ok)
}

// A path is used as it is here. What that costs when it is not is the whole of
// the other file: an empty translator refuses rather than returning the path
// unchanged, so a caller cannot silently get a Windows path into a hook by
// forgetting to find one.
func TestNothingIsTranslatedByAnEmptyTranslator(t *testing.T) {
	_, err := Cygpath{}.ToUnix(t.Context(), "/home/example/.bashrc")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no path translator")
}
