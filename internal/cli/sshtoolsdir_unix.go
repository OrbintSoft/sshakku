//go:build unix

package cli

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

// sessionSSHToolsDir names nothing here. There is one OpenSSH on such a system
// and the agent is a socket any build of it can open, so a session already runs
// tools that reach the agent it is pointed at — and putting a directory ahead
// of somebody's own PATH to tell them what they already have would be a change
// with no outcome.
func sessionSSHToolsDir(context.Context, shell.Dialect) (string, error) { return "", nil }
