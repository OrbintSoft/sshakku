//go:build unix

package cli

// crossUserDiagnosisHere is this system's answer about reporting on an account
// other than the one running the command.
//
// Implemented here: an account is a numeric uid, which is what `--user`
// resolves, what `SUDO_UID` carries, and what the per-login socket token is kept
// under — so a session other than the caller's own can be named, found and read.
// There is nothing to refuse, and no refusal to word.
var crossUserDiagnosisHere = crossUserDiagnosis{implemented: true}
