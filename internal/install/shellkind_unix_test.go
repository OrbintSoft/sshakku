//go:build unix

package install

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This build reads no process's image path, and the false it answers with is
// what makes a caller fall back to the name the process table gave — which here
// is the right answer rather than a degraded one, since every name in this
// system's table settles the question by itself.
//
// Asked about this very process, so a build that started answering would be
// caught giving a path rather than by nobody noticing.
func TestThisSystemAnswersNothingAboutAProcessImage(t *testing.T) {
	path, ok := ImagePath(os.Getpid())

	assert.False(t, ok, "and a caller reads that as having to use the name instead")
	assert.Empty(t, path, "a false with a path beside it would be a caller's invitation to use it")
}
