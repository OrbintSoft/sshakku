// Package paths computes and creates sshakku's per-user runtime layout: config
// under the XDG config dir, the session log under the XDG state dir, the agent
// socket in the per-user tmpfs — always outside ~/.ssh, which is OpenSSH's
// domain.
package paths

import (
	"path/filepath"
	"strconv"
)

const app = "sshakku"

// Env holds the environment inputs to the path computation, so Resolve stays a
// pure function that is easy to test.
type Env struct {
	Home       string // $HOME.
	ConfigHome string // $XDG_CONFIG_HOME (may be empty).
	StateHome  string // $XDG_STATE_HOME (may be empty).
	RuntimeDir string // $XDG_RUNTIME_DIR (may be empty).
	CacheHome  string // $XDG_CACHE_HOME (may be empty).
	// TempDir is the per-user temporary directory this session was given, and
	// only if it is private to this user — a shared one is left out here
	// rather than rejected later, so nothing downstream has to know the
	// difference. Empty when there is none to use.
	TempDir string
	UID     int
}

// Layout is the set of resolved paths.
type Layout struct {
	ConfigDir  string
	StateDir   string
	RuntimeDir string
	SocketDir  string // RuntimeDir; a per-login token component is added later.
	AgentSock  string
	AgentLock  string
	LogFile    string
	// RuntimeDirRefused names a runtime directory the environment pointed at
	// and this layout did not use, because it is there but is not one this
	// account has to itself. A directory that is merely absent is not refused:
	// nothing was turned down, so there is nothing to report. Empty in both of
	// those cases, and it is never a path anything is computed from — it exists
	// so the session log and the report can name what was asked for.
	RuntimeDirRefused string
}

// Resolve computes the layout from env. probe reports whether a directory is
// there; requirePrivate additionally asks that it be one this account has to
// itself, which is asked of every candidate for the socket's home.
//
// It is asked of what the environment named most of all, not least: coming from
// the environment is what makes a directory worth doubting rather than what
// settles it. $XDG_RUNTIME_DIR is the one input to these paths that can still
// be naming another account's directory — which is what a shell carries when it
// becomes another user without opening a session of its own — and the socket
// put there is the front door to every key the session loads.
func Resolve(env Env, probe func(path string, requirePrivate bool) bool) Layout {
	configHome := env.ConfigHome
	if configHome == "" {
		configHome = filepath.Join(env.Home, ".config")
	}
	configDir := filepath.Join(configHome, app)

	stateHome := env.StateHome
	if stateHome == "" {
		stateHome = filepath.Join(env.Home, ".local", "state")
	}
	stateDir := filepath.Join(stateHome, app)

	runtimeDir, runtimeDirRefused := resolveRuntimeDir(env, probe)
	socketDir := runtimeDir // a per-login token is inserted here in a later step.

	return Layout{
		ConfigDir:  configDir,
		StateDir:   stateDir,
		RuntimeDir: runtimeDir,
		SocketDir:  socketDir,
		AgentSock:  filepath.Join(socketDir, "agent.sock"),
		AgentLock:  filepath.Join(socketDir, ".start.lock"),
		LogFile:    filepath.Join(stateDir, "sessions.log"),

		RuntimeDirRefused: runtimeDirRefused,
	}
}

// WithSocketToken inserts a per-login token as a socket-path component, so the
// socket path is not reproducible across logins or reboots. An empty token (no
// keyring available) leaves the layout unchanged — a tokenless degradation.
func (l Layout) WithSocketToken(token string) Layout {
	if token == "" {
		return l
	}
	l.SocketDir = filepath.Join(l.RuntimeDir, token)
	l.AgentSock = filepath.Join(l.SocketDir, "agent.sock")
	l.AgentLock = filepath.Join(l.SocketDir, ".start.lock")
	return l
}

// resolveRuntimeDir picks the per-user tmpfs base, independent of the desktop or
// display server: XDG_RUNTIME_DIR, then its canonical /run/user/$UID, then the
// session's own private temporary directory, and last a private dir under
// $HOME. Every candidate must be one this account has to itself; the first that
// is, wins.
//
// refused names the directory the environment pointed at where that directory
// is there and is not this account's alone. A directory that is merely absent
// does not fill it: nothing was turned down, there is nothing to look into, and
// a report saying otherwise would send somebody after a stale variable as
// though it were somebody else's directory.
//
// The temporary directory comes before the home because the agent socket's
// address is bound by the kernel to barely a hundred bytes, while a home
// directory has no such limit and contributes its whole length: on a system
// with neither of the first two — macOS, a container, anything without logind
// — a home of ordinary depth still fits, and a long one leaves the session
// with no agent at all. It is also where that system's own ssh-agent puts its
// socket when nobody tells it otherwise.
func resolveRuntimeDir(env Env, probe func(string, bool) bool) (dir, refused string) {
	if env.RuntimeDir != "" && probe(env.RuntimeDir, false) {
		if probe(env.RuntimeDir, true) {
			return filepath.Join(env.RuntimeDir, app), ""
		}
		refused = env.RuntimeDir
	}
	runUser := filepath.Join("/run/user", strconv.Itoa(env.UID))
	if probe(runUser, true) {
		return filepath.Join(runUser, app), refused
	}
	if env.TempDir != "" {
		return filepath.Join(env.TempDir, app), refused
	}
	cacheHome := env.CacheHome
	if cacheHome == "" {
		cacheHome = filepath.Join(env.Home, ".cache")
	}
	return filepath.Join(cacheHome, app), refused
}
