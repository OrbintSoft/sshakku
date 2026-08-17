package platform

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What this package exists for is that a caller can act on the answer. A
// message cannot be acted on; the sentinel can.
func TestSomethingUnimplementedIsRecognisedByTheSentinelAndNotByItsWords(t *testing.T) {
	err := Unimplemented("keeping an ssh-agent on a fixed endpoint")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnimplemented)
	assert.Contains(t, err.Error(), "keeping an ssh-agent on a fixed endpoint",
		"and the words still name the piece, for whoever reads the log")
	assert.Contains(t, err.Error(), runtime.GOOS,
		"and which system it is about, since another one may well have it")
}

// The distinction the package doc claims: a genuine refusal from the standard
// library must not be mistaken for a piece this project has yet to write.
func TestARefusalFromElsewhereIsNotMistakenForSomethingUnwritten(t *testing.T) {
	refused := fmt.Errorf("changing the mode of a file: %w", errors.ErrUnsupported)

	assert.NotErrorIs(t, refused, ErrUnimplemented)
	assert.NotErrorIs(t, Unimplemented("keeping an ssh-agent"), errors.ErrUnsupported)
}
