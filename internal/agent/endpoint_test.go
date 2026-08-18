package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// F50: the two writings name one agent. The pipe is written as its own system
// writes it, and a POSIX-emulating shell is handed the same name with the
// separators turned round, because that is the form that arrives intact when
// such a shell hands its environment to a native program.
func TestPipeEndpointIsWrittenForBothKindsOfShell(t *testing.T) {
	e := PipeEndpoint(`\\.\pipe\openssh-ssh-agent`)

	assert.Equal(t, `\\.\pipe\openssh-ssh-agent`, e.Native(),
		"the system's own shells read the writing that system uses")
	assert.Equal(t, "//./pipe/openssh-ssh-agent", e.ForPosixShell(),
		"a POSIX-emulating shell is handed the writing it can carry")
}

// F50: whatever the name, the two writings differ only in their separators —
// so a reader comparing what a shell holds against what SSHakku reports can
// tell that both name the one agent.
func TestBothWritingsDifferOnlyInTheirSeparators(t *testing.T) {
	e := PipeEndpoint(`\\.\pipe\sshakku-something-else`)

	assert.Equal(t, strings.ReplaceAll(e.Native(), `\`, "/"), e.ForPosixShell(),
		"one name, written two ways")
	assert.NotContains(t, e.ForPosixShell(), `\`,
		"a separator left behind is one the shell would eat on the way through")
}
