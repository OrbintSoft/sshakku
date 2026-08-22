//go:build windows

package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// F50: the endpoint this platform names is a pipe, and it is offered in both
// writings. That it is the pipe the system is really serving is not something
// this test can tell — that question is asked of the agent itself, by dialling
// it, and is answered where the probe lives.
func TestSystemEndpointIsAPipeInBothWritings(t *testing.T) {
	e := SystemEndpoint()

	assert.True(t, strings.HasPrefix(e.Native(), `\\.\pipe\`),
		"the system's own shells are handed a pipe name as this system writes one")
	assert.True(t, strings.HasPrefix(e.ForPosixShell(), "//./pipe/"),
		"a POSIX-emulating shell is handed the writing that survives its environment")
	assert.NotContains(t, e.ForPosixShell(), `\`,
		"a separator left behind is one such a shell would eat on the way through")
}
