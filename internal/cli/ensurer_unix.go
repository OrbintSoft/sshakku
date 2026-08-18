//go:build unix

package cli

import (
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
	"github.com/OrbintSoft/sshakku/internal/agent/reach"
)

// agentLockWait bounds how long a login blocks for the start lock before it
// proceeds without it, so a stuck holder slows the login but never hangs it.
const agentLockWait = 5 * time.Second

// platformEnsurer is the agent lifecycle this system has: an ssh-agent this
// program starts on a socket of its own choosing and keeps healthy on a fixed
// path, reaping what has died and adopting what it did not start.
func platformEnsurer() agentEnsurer {
	return agent.Manager{
		Prober:    reach.SocketProber{},
		Inspector: inspect.Inspector{},
		Runner:    agent.ExecRunner{},
		Signaler:  agent.SysSignaler{},
		Locker:    agent.FlockLocker{Wait: agentLockWait},
	}
}
