// Package agent tends the user's ssh-agent: it starts one on the fixed socket,
// reaps dead agents and sockets, and adopts a healthy agent started by
// something else. It never reimplements the agent — it only manages the
// lifecycle of OpenSSH's ssh-agent.
package agent

import "context"

// Prober reports whether a usable ssh-agent answers on a unix socket path.
// Reachable mirrors `ssh-add -l`: an agent with zero keys is still healthy.
type Prober interface {
	Reachable(ctx context.Context, socket string) bool
}
