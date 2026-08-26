//go:build unix

package agent

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/platform"
)

// EnableAgentService reports that there is no service here to enable.
//
// The agent on this system is a process this program starts, so nothing about
// it can be disabled in the way a service can. Callers reach this only by
// having been told a service was disabled, which nothing on this system says —
// so it answers rather than pretending to have done something.
func EnableAgentService(context.Context) error {
	return platform.Unimplemented("enabling the agent's service")
}
