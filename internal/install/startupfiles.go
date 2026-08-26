package install

import (
	"errors"
	"path"
)

// loginFiles are the files a login shell looks for, in the order it looks. It
// reads the first one it finds and no others.
var loginFiles = []string{".bash_profile", ".bash_login", ".profile"}

// BourneLoginFile names the file a Bourne shell will really read when it logs
// in, which is the primary wiring point: under Git Bash, and after a console
// login, it is the one that fires. It returns every file that shell could have
// read as well, because that is what an uninstall has to sweep: the choice below
// depends on which of them existed at the time, so an uninstall cannot reach the
// same answer by making it again.
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
func BourneLoginFile(home string, exists func(path string) bool) (chosen string, candidates []string, err error) {
	candidates = make([]string, 0, len(loginFiles))
	for _, name := range loginFiles {
		candidate, err := under(home, name)
		if err != nil {
			return "", nil, err
		}
		candidates = append(candidates, candidate)
	}

	for _, candidate := range candidates {
		if exists != nil && exists(candidate) {
			return candidate, candidates, nil
		}
	}
	return candidates[0], candidates, nil
}

// BourneRCFile names the file a Bourne shell reads when it starts interactive
// without logging in — a new terminal tab, a multiplexer pane. Wiring it is
// additive and never a replacement for the login file.
func BourneRCFile(home string) (string, error) {
	return under(home, ".bashrc")
}

// errShellNamedNoHomeForStartup is a home directory that is not there when a
// startup file has to be named under it. Joining onto an empty home would name
// a path relative to wherever the install was run.
var errShellNamedNoHomeForStartup = errors.New("the shell named no home directory, so there is no startup file to name")

// under joins a name to a home directory in the shell's spelling.
//
// path.Join, never filepath.Join. These paths belong to the shell, not to the
// program: this program runs on a system whose separator may be a backslash,
// and a startup file joined with one names nothing the shell can open — while
// on the system where that matters the mistake is invisible, because the file
// gets created under the name it was asked for and simply never read.
func under(home, name string) (string, error) {
	if home == "" {
		return "", errShellNamedNoHomeForStartup
	}
	return path.Join(home, name), nil
}
