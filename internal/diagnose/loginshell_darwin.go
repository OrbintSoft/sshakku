//go:build darwin

package diagnose

// loginShellHint explains the most common reason a shell never picked up
// sshakku's wiring: it was never a login shell, so the profile file that
// sources sshakku's hook was never read. Terminal.app and iTerm2 start a
// login shell by default, but many others don't — an IDE's integrated
// terminal, tmux, screen.
const loginShellHint = "likely because this shell was never a login shell, so /etc/zprofile " +
	"(or ~/.zprofile) was never sourced — opening a plain new terminal tab won't fix it unless " +
	"that terminal also starts a login shell; re-source your profile directly, or start a login " +
	"shell explicitly (e.g. zsh -l)"
