package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/platform"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// agentEnsurer drives the endpoint to a healthy ssh-agent. The production
// implementation is whichever lifecycle this system has; a fake stands in so
// runEnsure's result handling is exercised without spawning, reaping, adopting
// or starting anything on the test host.
type agentEnsurer interface {
	EnsureAgent(ctx context.Context, cfg agent.EnsureConfig, log agent.Logger) (agent.EnsureResult, error)
}

// realEnsurer wires the concrete system probes into the lifecycle this system
// has; deps.ensurer holds the result so command bodies can be tested against a
// fake.
func realEnsurer() agentEnsurer {
	return ensurerFor(agent.KeepsAgents())
}

// ensurerFor is what drives the agent on a system that can keep one, and what
// answers for a system that cannot.
//
// Where this build cannot keep an agent at all, what a lifecycle composes
// itself out of does not exist, and the honest answer is the one that says so
// rather than a lifecycle that would fail at its first step. Which system this
// is comes in as the answer it is, so both halves stay checkable from either
// machine; which lifecycle a system that can keep one has is that system's own
// (see platformEnsurer).
func ensurerFor(keepsAgents bool) agentEnsurer {
	if !keepsAgents {
		return agent.NoMechanism{}
	}
	return platformEnsurer()
}

// shellInit resolves and creates the per-user runtime layout, drives the fixed
// socket to a healthy ssh-agent, then prints the result as shell assignments for
// the login entrypoint to eval:
//
//	agent_sock='…'
//	agent_lock='…'
//	log_file='…'
//
// agent_sock is the live socket EnsureAgent settled on, which may be an adopted
// agent rather than the fixed path. Only these assignments go to stdout;
// diagnostics and anomalies go to stderr and the session log.
//
// A session is also where a key that has run out of time is taken back out of
// the agent, on a system whose agent expires nothing itself (see expireKeys).
//
// --shell says which language to print them in (see shell.FromArgs); a shell
// that says nothing gets the Bourne form above. An invocation that cannot be
// printed for is answered before the agent is touched: it is a mistake in what
// was asked, and starting an agent is not part of answering one.
func (d deps) shellInit(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	dialect, err := shell.FromArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: shell-init: %v\n", err)
		return 2
	}

	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		// Best-effort: the log dir may be the very thing we failed to create.
		_ = sessionlog.New(layout.LogFile).Log("ERROR", fmt.Sprintf("shell-init: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	paths.CleanupLegacyAgentDir(env.Home)

	live, code := d.runEnsure(ctx, stderr, env, layout)
	if code != 0 {
		return code
	}
	d.expireKeys(ctx, layout, live)

	assignments := []struct{ name, value string }{
		{"agent_sock", endpointFor(dialect, live)},
		{"agent_lock", layout.AgentLock},
		{"log_file", layout.LogFile},
	}
	for _, a := range assignments {
		if _, err := io.WriteString(stdout, dialect.SetVar(a.name, a.value)); err != nil {
			_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
			return 1
		}
	}
	return 0
}

// ensureAgent resolves the runtime layout, drives the fixed socket to a healthy
// ssh-agent (starting, reaping, or adopting as needed), and prints the live
// socket as a shell assignment:
//
//	agent_sock='…'
//
// It is a standalone entry point for exercising the lifecycle; the login path
// reaches the same logic through shell-init, which adds the other assignments.
// --shell chooses the language, exactly as it does there.
func (d deps) ensureAgent(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	dialect, err := shell.FromArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: ensure-agent: %v\n", err)
		return 2
	}

	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}

	live, code := d.runEnsure(ctx, stderr, env, layout)
	if code != 0 {
		return code
	}
	if _, err := io.WriteString(stdout, dialect.SetVar("agent_sock", endpointFor(dialect, live))); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	return 0
}

// keystateDir is where load-keys records each key's added-at/lifetime state,
// so doctor can later report it; shared so both sides agree on the path. It
// sits alongside the giveup dir, under the per-login runtime directory
// (tmpfs, wiped on logout/reboot — see internal/keystate).
func keystateDir(layout paths.Layout) string {
	return filepath.Join(filepath.Dir(layout.AgentSock), "keystate")
}

// endpointFor gives the endpoint in the writing the shell being printed for
// reads. A system whose agent listens on a socket has one writing and hands it
// to every shell; one that reaches its agent through a named pipe has two, and
// which of them arrives intact depends on the shell carrying it.
func endpointFor(dialect shell.Dialect, live agent.Endpoint) string {
	if dialect.Name() == shell.Posix {
		return live.ForPosixShell()
	}
	return live.Native()
}

// runEnsure drives the fixed endpoint to a healthy ssh-agent for the resolved
// layout, serialising concurrent logins on the start lock and reporting
// anomalies and errors to stderr and the session log. It returns the live
// endpoint to expose and a process exit code (0 on success). shell-init and
// ensure-agent share it so the login path and the standalone command drive the
// agent identically; each caller prints the assignments it needs.
func (d deps) runEnsure(ctx context.Context, stderr io.Writer, env paths.Env, layout paths.Layout) (agent.Endpoint, int) {
	log := sessionlog.New(layout.LogFile)
	cfg := agent.EnsureConfig{
		FixedSock: layout.AgentSock,
		LegacyDir: filepath.Join(env.Home, ".ssh", "agent"),
		StatePath: filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		LockPath:  layout.AgentLock,
		OurUID:    env.UID,
	}

	res, err := d.ensurer.EnsureAgent(ctx, cfg, log)
	if err != nil {
		// A mechanism this build has none of on this system is not this
		// session's failure. The shell opens, with whatever paths do exist,
		// and the absence is written down once as the fact it is — where an
		// error on the terminal would be read as something to go and fix, and
		// there is nothing here to fix.
		if errors.Is(err, platform.ErrUnimplemented) {
			_ = log.Log("INFO", fmt.Sprintf("ensure-agent: %v", err))
			return agent.Endpoint{}, 0
		}
		_ = log.Log("ERROR", fmt.Sprintf("ensure-agent: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return agent.Endpoint{}, 1
	}
	if res.Anomaly != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s\n", res.Anomaly)
	}
	return res.Live, 0
}
