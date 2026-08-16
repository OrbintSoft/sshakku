package agent

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/platform"
)

// NoMechanism is what drives the agent on a system where this build has no way
// to keep one on a fixed endpoint.
//
// It answers once, up front, instead of letting Manager walk into whichever
// piece happens to be missing first — the lock, the process enumeration, the
// signal. Which of those a caller met would be an accident of the order the
// steps run in, and would read as a defect in that step rather than as the
// truth, which is that none of it is written here yet.
type NoMechanism struct{}

// EnsureAgent reports that no agent can be kept on this system.
func (NoMechanism) EnsureAgent(context.Context, EnsureConfig, Logger) (EnsureResult, error) {
	return EnsureResult{}, platform.Unimplemented("keeping an ssh-agent on a fixed endpoint")
}
