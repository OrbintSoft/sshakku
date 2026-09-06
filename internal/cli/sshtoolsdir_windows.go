//go:build windows

package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/OrbintSoft/sshakku/internal/install"
	"github.com/OrbintSoft/sshakku/internal/sshtools"
)

// The two ways a session that is running the wrong ssh cannot be given the
// right one. Both leave the session as it was — with an endpoint it cannot
// open — so both are said rather than passed over.
var (
	errNoBuildForThisSession = errors.New("this session's ssh is built for a POSIX emulation layer and cannot" +
		" reach this system's agent, and this machine has no build of OpenSSH that can")
	errNoTranslatorForThisSession = errors.New("this session spells paths its own way and ships no translator" +
		" to say how, so there is no writing of that directory it could be given")
)

// sessionSSHToolsDir names the directory this session has to search ahead of
// its own PATH so that the ssh it runs reaches the agent it has just been
// pointed at, or "" where the ssh it already runs does.
//
// The endpoint and the tools go together. A session handed an endpoint its own
// ssh cannot open has been pointed at nothing: `ssh` typed there, and the one
// `git` starts on the user's behalf, ask for a passphrase however full the
// agent is. What goes in front is the whole directory rather than the two
// programs SSHakku itself drives, so the ssh, scp and sftp that session runs
// all belong to the same OpenSSH — a session running one build's ssh and
// another's scp is a stranger arrangement than either.
//
// The writing depends on who is being told. A shell that emulates POSIX has a
// root of its own and reads no path this program would write, and which
// spelling is which is that environment's business: it ships the answer as a
// program, and the program is asked rather than the mapping guessed at. The
// translator is looked for beside the very build that was passed over, since
// that build is the one thing already known to belong to that environment.
func sessionSSHToolsDir(ctx context.Context, dialect shell.Dialect) (string, error) {
	tools := sshtools.ThisSystem().SessionSSHTools(sshtools.SSHName)
	switch {
	case tools.Emulated == "":
		return "", nil
	case tools.Native == "":
		return "", errNoBuildForThisSession
	case dialect.Name() != shell.Posix:
		return tools.Native, nil
	}

	translator, found := install.FindCygpath(tools.Emulated)
	if !found {
		return "", errNoTranslatorForThisSession
	}
	spelled, err := translator.ToUnix(ctx, tools.Native)
	if err != nil {
		return "", fmt.Errorf("spelling %s the way this session reads one: %w", tools.Native, err)
	}
	return spelled, nil
}
