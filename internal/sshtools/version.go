package sshtools

import (
	"context"
	"errors"

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

// errNoVersion is what an answer that is not a version is refused with. A
// build whose version could not be read is not an old build, and the two must
// never collapse into one another.
var errNoVersion = errors.New("no OpenSSH version in what the program answered")

// Version is what one build of OpenSSH says it is when asked.
//
// Major and Minor are the release; the portable suffix OpenSSH puts after them
// (`p1`, `p2`) is not part of it and is not kept. Text is the whole line the
// build printed, so a report can quote what was said rather than a number
// reconstructed from it.
//
// The zero value means no version was read, which is a third answer and not a
// very old one — see Unread.
//
// Nothing below is implemented yet: the tests beside this file say what each
// of them has to do, and say it by failing.
type Version struct {
	Major int
	Minor int
	Text  string
}

// Label is how the build names itself, without the crypto library it was
// linked against.
func (v Version) Label() string {
	return ""
}

// Unread reports whether no version was read at all.
func (v Version) Unread() bool {
	return false
}

// CanBeToldToAsk reports whether this build understands being pointed at a
// passphrase helper it would not have used on its own.
func (v Version) CanBeToldToAsk() bool {
	return false
}

// ParseVersion reads the release out of the line an OpenSSH build prints when
// asked for its version, in any of the spellings OpenSSH uses — including the
// name Microsoft's build gives itself. An answer with no version in it is an
// error rather than a zero.
func ParseVersion(printed string) (Version, error) {
	_ = printed
	return Version{}, nil
}

// ReadVersion asks the named OpenSSH program what it is. OpenSSH answers `-V`
// on standard error, which is where this reads it from.
func ReadVersion(ctx context.Context, r run.Runner, prog string) (Version, error) {
	_, _ = ctx, r
	_ = prog
	return Version{}, nil
}
