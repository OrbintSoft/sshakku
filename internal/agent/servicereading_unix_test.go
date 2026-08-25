//go:build unix

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F55 describes a service, and this system has none to describe: the agent here
// is a process this program starts on a socket it chose, so there is no service
// manager to ask and nothing for the report to print. Saying so is the answer,
// not a gap — a reading that named some service would have the report claiming
// something about a machine that has not got one.
func TestASystemWhoseAgentIsAProcessDescribesNoService(t *testing.T) {
	reading := ReadAgentService(t.Context())

	assert.False(t, reading.ServedByAService(), "the agent here is a process, not a service")
	assert.Empty(t, reading.Name, "there is no service to name")
	require.NoError(t, reading.Err, "having no service is not a failure to read one")
}
