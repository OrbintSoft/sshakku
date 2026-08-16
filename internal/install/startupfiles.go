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

// loginFiles are the files a login shell looks for, in the order it looks. It
// reads the first one it finds and no others.
var loginFiles = []string{".bash_profile", ".bash_login", ".profile"}

// BourneLoginFile names the file a Bourne shell will really read when it logs
// in, which is the primary wiring point: under Git Bash, and after a console
// login, it is the one that fires.
//
// The file is chosen by looking, and that is not fussiness. A login shell reads
// the first of the three above that exists and no others, so writing a hook
// into .bash_profile on an account set up with .profile does not add SSHakku to
// that account's configuration — it replaces it. The user's own login setup
// stops running entirely, silently, and they find out at their next login by
// everything being gone. Measured against a real shell: with only .profile it
// is read; the moment .bash_profile exists, .profile is not read at all.
//
// Where none of them exists there is nothing to displace, and the first is
// created — which is the name that shell looks for first.
//
// exists is passed in because on the system this matters for these paths are in
// the shell's spelling and this program cannot look at one directly; the caller
// translates. It is also what lets the choice be checked without a filesystem.
func BourneLoginFile(home string, exists func(path string) bool) (string, error) {
	for _, name := range loginFiles {
		candidate, err := under(home, name)
		if err != nil {
			return "", err
		}
		if exists != nil && exists(candidate) {
			return candidate, nil
		}
	}
	return under(home, loginFiles[0])
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
