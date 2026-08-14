//go:build windows

package diagnose

// wantSSHLauncher is how a report on this system describes an agent an SSH
// login started. This build reads no process tree, so its table recognises no
// launcher by name (ancestry_windows.go) and startedBy falls back to naming
// the immediate parent — which is still an attribution, just not one that says
// what that parent is for.
const wantSSHLauncher = "sshd"
