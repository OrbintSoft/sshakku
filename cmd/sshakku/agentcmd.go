package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
)

// agentLockWait bounds how long a login blocks for the start lock before it
// proceeds without it, so a stuck holder slows the login but never hangs it.
const agentLockWait = 5 * time.Second

// agentEnsurer drives the fixed socket to a healthy ssh-agent. The production
// implementation is the concrete agent.Manager; a fake stands in so runEnsure's
// result handling is exercised without spawning, reaping, or adopting a real
// agent on the test host.
type agentEnsurer interface {
	EnsureAgent(cfg agent.EnsureConfig, log agent.Logger) (agent.EnsureResult, error)
}

// realEnsurer wires the concrete system probes and runners into the production
// agent.Manager. It composes real implementations and has no branching logic of
// its own; deps.ensurer holds the result so command bodies can be tested against
// a fake.
func realEnsurer() agentEnsurer {
	return agent.Manager{
		Prober:    agent.SocketProber{},
		Inspector: agent.Inspector{},
		Runner:    agent.ExecRunner{},
		Signaler:  agent.SysSignaler{},
		Locker:    agent.FlockLocker{Wait: agentLockWait},
	}
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
func (d deps) shellInit(stdout, stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		// Best-effort: the log dir may be the very thing we failed to create.
		_ = sessionlog.New(layout.LogFile).Log("ERROR", fmt.Sprintf("shell-init: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}
	paths.CleanupLegacyAgentDir(env.Home)

	liveSock, code := d.runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}

	assignments := []struct{ name, value string }{
		{"agent_sock", liveSock},
		{"agent_lock", layout.AgentLock},
		{"log_file", layout.LogFile},
	}
	for _, a := range assignments {
		if _, err := fmt.Fprintf(stdout, "%s=%s\n", a.name, shellSingleQuote(a.value)); err != nil {
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
func (d deps) ensureAgent(stdout, stderr io.Writer) int {
	env := paths.FromOS()
	layout := paths.Resolve(env, paths.ProbeDir).WithSocketToken(paths.SocketToken())
	if err := paths.Ensure(layout); err != nil {
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return 1
	}

	liveSock, code := d.runEnsure(stderr, env, layout)
	if code != 0 {
		return code
	}
	if _, err := fmt.Fprintf(stdout, "agent_sock=%s\n", shellSingleQuote(liveSock)); err != nil {
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

// runEnsure drives the fixed socket to a healthy ssh-agent for the resolved
// layout, serialising concurrent logins on the start lock and reporting
// anomalies and errors to stderr and the session log. It returns the live socket
// to expose and a process exit code (0 on success). shell-init and ensure-agent
// share it so the login path and the standalone command drive the agent
// identically; each caller prints the assignments it needs.
func (d deps) runEnsure(stderr io.Writer, env paths.Env, layout paths.Layout) (string, int) {
	log := sessionlog.New(layout.LogFile)
	cfg := agent.EnsureConfig{
		FixedSock: layout.AgentSock,
		LegacyDir: filepath.Join(env.Home, ".ssh", "agent"),
		StatePath: filepath.Join(filepath.Dir(layout.AgentSock), "agent.state"),
		LockPath:  layout.AgentLock,
		OurUID:    env.UID,
	}

	res, err := d.ensurer.EnsureAgent(cfg, log)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("ensure-agent: %v", err))
		_, _ = fmt.Fprintf(stderr, "sshakku: %v\n", err)
		return "", 1
	}
	if res.Anomaly != "" {
		_, _ = fmt.Fprintf(stderr, "sshakku: %s\n", res.Anomaly)
	}
	return res.LiveSock, 0
}
