package install

import (
	"errors"
	"path"
)

// MachineDropInDir is the directory a Bourne shell reads at every login, for
// every account, in the shell's own spelling.
//
// Under a POSIX-emulating environment this is not the Windows system's `/etc`
// but the environment's own, wherever it was installed. That is the point of
// naming it this way: translated by that environment's own translator it comes
// back as the real directory, and no installation path is assumed here.
const MachineDropInDir = "/etc/profile.d"

// BourneLoginFile names the file a Bourne shell reads when it logs in, which is
// the primary wiring point: under Git Bash and after a console login it is the
// one that fires.
func BourneLoginFile(home string) (string, error) {
	return under(home, ".bash_profile")
}

// BourneRCFile names the file a Bourne shell reads when it starts interactive
// without logging in — a new terminal tab, a multiplexer pane. Wiring it is
// additive and never a replacement for the login file.
func BourneRCFile(home string) (string, error) {
	return under(home, ".bashrc")
}

// under joins a name to a home directory in the shell's spelling.
//
// path.Join, never filepath.Join. These paths belong to the shell, not to the
// program: this program runs on a system whose separator may be a backslash,
// and a startup file joined with one names nothing the shell can open — while
// on the system where that matters the mistake is invisible, because the file
// gets created under the name it was asked for and simply never read.
func under(home, name string) (string, error) {
	if home == "" {
		return "", errors.New("the shell named no home directory, so there is no startup file to name")
	}
	return path.Join(home, name), nil
}
