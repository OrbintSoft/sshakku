//go:build windows

package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// What this prompter says about itself needs no window and no screen, so it is
// asked here rather than beside the tests that draw one.
func TestTheBoxNeedsNothingInstalled(t *testing.T) {
	assert.True(t, NativePrompter{}.Available(t.Context()),
		"SSHakku draws this box itself, so there is never anything missing to draw it with")
	assert.Equal(t, "native", NativePrompter{}.Name(),
		"the name is what gui_prompter calls it, so a message about it names something the user can write")
}
