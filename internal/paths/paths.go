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

// Need is what a directory has to be before something of ours goes into it.
// There are two questions and not one because the stakes differ by directory,
// and asking the stricter one everywhere would turn away directories that are
// plainly the user's own.
type Need int

const (
	// NeedThere asks only that the path be a directory at all.
	NeedThere Need = iota
	// NeedUnwritable asks that this account owns it and nobody else may write
	// it. It is the question for a directory whose contents we read or keep —
	// a configuration, a record — because what would harm us there is somebody
	// replacing what is inside or renaming it away, and both of those are
	// governed by write permission. Being merely visible to others is not a
	// wrong: ~/.config and ~/.cache are mode 0755 on an ordinary system, and a
	// build that refused them would discard a user's own settings for a
	// permission that gives nobody else anything.
	NeedUnwritable
	// NeedPrivate asks that nobody else may even enter it. It is the question
	// for the directory the socket's home is chosen from, where the convention
	// the system itself follows is 0700 (/run/user/<uid>), so insisting on it
	// turns away nothing that was going to be used anyway.
	NeedPrivate
)

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
	// OwnerUnknowable says this build cannot establish who a directory belongs
	// to at all, and so must turn none of them down: an unanswered question is
	// not a refusal, and a build that discarded a user's own configuration for
	// want of an answer would fail them worse than the thing being guarded
	// against, in a way they could do nothing about. It is phrased as the
	// exception rather than the rule so that an Env nobody filled in refuses a
	// stranger's directory instead of accepting it. See PrivateDir.
	OwnerUnknowable bool
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
	// Refused holds the directories the environment pointed at and this layout
	// did not use, because each is there but is not one this account has to
	// itself. A directory that is merely absent is not refused: nothing was
	// turned down, so there is nothing to report. None of these is ever a path
	// anything is computed from — they exist so the session log and the report
	// can name what was asked for, which is the difference between a session
	// whose files quietly moved and one that can be worked back from.
	Refused []Refusal
}

// Refusal is one directory that was named and not used. It carries the
// variable as well as the path because a report that quoted the wrong variable
// would send a reader to edit something that had nothing to do with it.
type Refusal struct {
	Var  string
	Path string
}

// Resolve computes the layout from env. probe reports whether a directory is
// there; requirePrivate additionally asks that it be one this account has to
// itself, which is asked of every directory the environment names.
//
// It is asked of what the environment named most of all, not least: coming from
// the environment is what makes a directory worth doubting rather than what
// settles it. Every one of these variables is carried in by a shell that
// becomes another user without opening a session of its own, and each leads
// somewhere worth reaching — the socket is the front door to every key the
// session loads, the configuration decides where passphrases are filed and
// which command is run to edit it, and the state directory takes the record of
// what was done with the keys.
func Resolve(env Env, probe func(path string, need Need) bool) Layout {
	pick := &chooser{env: env, probe: probe}

	configDir := filepath.Join(
		pick.ours("XDG_CONFIG_HOME", env.ConfigHome, filepath.Join(env.Home, ".config")), app)
	stateDir := filepath.Join(
		pick.ours("XDG_STATE_HOME", env.StateHome, filepath.Join(env.Home, ".local", "state")), app)

	runtimeDir := pick.runtimeDir()
	socketDir := runtimeDir // a per-login token is inserted here in a later step.

	return Layout{
		ConfigDir:  configDir,
		StateDir:   stateDir,
		RuntimeDir: runtimeDir,
		SocketDir:  socketDir,
		AgentSock:  filepath.Join(socketDir, "agent.sock"),
		AgentLock:  filepath.Join(socketDir, ".start.lock"),
		LogFile:    filepath.Join(stateDir, "sessions.log"),

		Refused: pick.refused,
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

// chooser picks each of the layout's directories and keeps what it turned
// down, so the session log and the report can name a directory that was asked
// for and not used.
type chooser struct {
	env     Env
	probe   func(string, Need) bool
	refused []Refusal
}

// ours returns the directory to use for one of the homes the layout is built
// on: the one the environment named, unless that is there and is not a
// directory this account has to itself, in which case this account's own is
// used and the other is recorded.
//
// A directory that is merely absent is not refused. A stale variable left over
// from a session that ended names a directory nobody turned down, and reporting
// it as somebody else's would send a person looking for an intruder where there
// is only a path that no longer exists.
//
// Nor is anything refused where ownership cannot be established at all. These
// homes differ from the socket's in having nowhere else to go: a socket turned
// away from one directory is opened in the next, and it is the same socket, but
// a configuration turned away is a configuration not read, and the settings a
// user wrote are simply gone. Refusing what could only be doubted would cost
// them that for nothing.
func (c *chooser) ours(name, named, own string) string {
	if named == "" {
		return own
	}
	if c.env.OwnerUnknowable {
		return named
	}
	if c.theirs(name, named) {
		return own
	}
	return named
}

// theirs reports whether path is there and is not this account's alone,
// recording the refusal when it is.
func (c *chooser) theirs(name, path string) bool {
	if !c.probe(path, NeedThere) || c.probe(path, NeedUnwritable) {
		return false
	}
	c.refused = append(c.refused, Refusal{Var: name, Path: path})
	return true
}

// runtimeDir is where the socket goes: the directory the environment named if
// it is this account's alone, else the one logind makes, else the private
// temporary directory, else the cache.
//
// The ownership question is asked here whether or not it can be answered, which
// is the opposite of what ours does and is the difference between the two: the
// socket has somewhere else to go, so a directory that cannot be vouched for
// costs the session nothing but a different path, while the same doubt about a
// configuration would cost the user their settings.
//
// The first two candidates are asked for more than the last: the system makes
// /run/user/<uid> at 0700 and a session's own runtime directory is held to the
// convention the system itself follows, so insisting on it turns away nothing
// that was going to be used. The cache is a directory people keep other things
// in, at 0755 on an ordinary machine, and asking that of it would turn away a
// directory plainly the user's own — so it is asked what actually protects
// what we put there, which is that nobody else may write it.
//
// The private temporary directory is taken before the home because a socket
// address is bounded and a home directory is not: a home deep enough leaves a
// session with no agent it can reach at all.
func (c *chooser) runtimeDir() string {
	if c.env.RuntimeDir != "" && c.probe(c.env.RuntimeDir, NeedThere) {
		if c.probe(c.env.RuntimeDir, NeedPrivate) {
			return filepath.Join(c.env.RuntimeDir, app)
		}
		c.refused = append(c.refused, Refusal{Var: "XDG_RUNTIME_DIR", Path: c.env.RuntimeDir})
	}
	runUser := filepath.Join("/run/user", strconv.Itoa(c.env.UID))
	if c.probe(runUser, NeedPrivate) {
		return filepath.Join(runUser, app)
	}
	if c.env.TempDir != "" {
		return filepath.Join(c.env.TempDir, app)
	}
	// The cache is a directory the socket can land in, so it is asked the same
	// question as the homes: a parent another account may write is one this
	// account's directory can be renamed out of and answered in place of,
	// whatever the mode on the directory itself.
	return filepath.Join(
		c.ours("XDG_CACHE_HOME", c.env.CacheHome, filepath.Join(c.env.Home, ".cache")), app)
}
