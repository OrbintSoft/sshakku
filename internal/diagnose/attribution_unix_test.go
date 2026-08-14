//go:build unix

package diagnose

// wantSSHLauncher is how a report on this system describes an agent an SSH
// login started. This platform's table recognises sshd by name, so startedBy
// answers with the label that table gives — see ancestry_linux.go and
// ancestry_darwin.go.
const wantSSHLauncher = "an SSH login session (sshd)"
