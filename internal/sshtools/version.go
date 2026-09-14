package sshtools

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// AskpassRequireMajor and AskpassRequireMinor are the OpenSSH release that
// first understood SSH_ASKPASS_REQUIRE, which is what lets a build be sent to
// a passphrase helper it would not otherwise have reached. Older builds reach
// one only where DISPLAY names an X server; a session that names none — an
// ordinary Windows console, a Mac without one, a machine reached over SSH —
// gets no helper at all from them, whatever is exported.
const (
	AskpassRequireMajor = 8
	AskpassRequireMinor = 4
)

// versionTimeout bounds asking a program what it is. Answering takes no
// filesystem, no network and no agent, so a build that has not answered in
// this long is one that is not going to.
const versionTimeout = 5 * time.Second

// errNoVersion is what an answer that is not a version is refused with. A
// build whose version could not be read is not an old build, and the two must
// never collapse into one another.
var errNoVersion = errors.New("no OpenSSH version in what the program answered")

// versionPattern matches the release in what an OpenSSH build prints for `-V`.
// The optional words between the name and the number are how a fork spells
// itself — Microsoft's build calls itself OpenSSH_for_Windows — and the
// portable suffix after the number (`p1`, `p2`) is deliberately left out: it
// counts releases of the portability layer, not of OpenSSH.
var versionPattern = regexp.MustCompile(`OpenSSH_(?:[A-Za-z]+_)*(\d+)\.(\d+)`)

// Version is what one build of OpenSSH says it is when asked.
//
// Major and Minor are the release; the portable suffix OpenSSH puts after them
// (`p1`, `p2`) is not part of it and is not kept. Text is the whole line the
// build printed, so a report can quote what was said rather than a number
// reconstructed from it.
//
// The zero value means no version was read, which is a third answer and not a
// very old one — see Unread.
type Version struct {
	Major int
	Minor int
	Text  string
}

// Label is how the build names itself, without the crypto library it was
// linked against. OpenSSH answers `-V` with both, separated by a comma; only
// the first identifies the build, and a report quoting the whole line hands a
// reader a library's release date to weigh a decision about ssh with.
func (v Version) Label() string {
	name, _, _ := strings.Cut(v.Text, ",")
	return strings.TrimSpace(name)
}

// Unread reports whether no version was read at all. No OpenSSH has ever had a
// major of zero, so the zero value is free to mean this.
func (v Version) Unread() bool {
	return v.Major == 0
}

// CanBeToldToAsk reports whether this build understands being pointed at a
// passphrase helper it would not have used on its own. A version nobody read
// is not reported as unable: that would tell a reader with a current OpenSSH
// to replace the one thing that was never the problem.
func (v Version) CanBeToldToAsk() bool {
	if v.Unread() {
		return true
	}
	if v.Major != AskpassRequireMajor {
		return v.Major > AskpassRequireMajor
	}
	return v.Minor >= AskpassRequireMinor
}

// ParseVersion reads the release out of the line an OpenSSH build prints when
// asked for its version, in any of the spellings OpenSSH uses — including the
// name Microsoft's build gives itself. An answer with no version in it is an
// error rather than a zero.
func ParseVersion(printed string) (Version, error) {
	m := versionPattern.FindStringSubmatch(printed)
	if m == nil {
		return Version{}, errNoVersion
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return Version{}, errNoVersion
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return Version{}, errNoVersion
	}
	return Version{Major: major, Minor: minor, Text: printed}, nil
}

// ReadVersion asks the named OpenSSH program what it is. OpenSSH answers `-V`
// on standard error, which is where this reads it from; a build that answers
// on standard output instead is read there rather than called unreadable.
func ReadVersion(ctx context.Context, r run.Runner, prog string) (Version, error) {
	res, err := r.Run(ctx, run.Cmd{Name: prog, Args: []string{"-V"}, Timeout: versionTimeout})
	if err != nil {
		return Version{}, err
	}
	said := strings.TrimSpace(string(res.Stderr))
	if said == "" {
		said = strings.TrimSpace(string(res.Stdout))
	}
	return ParseVersion(said)
}
