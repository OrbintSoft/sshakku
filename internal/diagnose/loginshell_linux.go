//go:build linux

package diagnose

// loginShellHint explains the most common reason a shell never picked up
// sshakku's wiring: it was never a login shell, so the profile file that
// sources sshakku's hook was never read.
const loginShellHint = "likely because this shell was never a login shell, so /etc/profile.d " +
	"(or ~/.bash_profile) was never sourced — opening a plain new terminal tab won't fix it " +
	"unless that terminal also starts a login shell; re-source your profile directly, or start " +
	"a login shell explicitly (e.g. bash -l)"
