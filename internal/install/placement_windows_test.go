//go:build windows

package install

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A drop-in directory this system will not even be asked about is reported, and
// not answered "no". Answered "no", the hook would go into the profile itself
// and the real problem would go unmentioned — so the difference between "there
// is none" and "this could not be looked at" is one an install has to keep.
//
// A name this system refuses outright is what makes the question unanswerable
// here, where other systems reach it by being denied the directory above.
func TestADropInDirectoryThisSystemWillNotBeAskedAboutIsNotAnsweredAbsent(t *testing.T) {
	unaskable := filepath.Join(t.TempDir(), "profile.d\x00hidden")

	found, err := isDir(unaskable)

	require.Error(t, err)
	assert.False(t, found, "a question that could not be asked has not been answered no")
	assert.Contains(t, err.Error(), "drop-in directory", "and says what was being looked for")
}
