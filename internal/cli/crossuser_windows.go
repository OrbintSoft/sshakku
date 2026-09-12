//go:build windows

package cli

// crossUserDiagnosisHere is this system's answer about reporting on an account
// other than the one running the command.
//
// Not implemented here. `--user` names an account by numeric uid and confirms
// that account's own socket by reading a per-login token kept under it; this
// system has neither, since an account here is a SID and there is no keyring the
// token could be in. What it would take is a design this build has not made —
// reaching another account's session means assuming its identity — rather than a
// line somebody forgot to write, so the flag says so instead of failing at the
// first thing that happens not to parse.
//
// The refusal names the flag, what is absent, and the command that reaches the
// same report: run as that account, `sshakku doctor` answers the question
// `--user` was asked to.
var crossUserDiagnosisHere = crossUserDiagnosis{
	refusal: "--user is not implemented on Windows: this build reports only on the session of the " +
		`account running it; run "sshakku doctor" as that account instead`,
}
