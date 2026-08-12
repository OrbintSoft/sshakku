//go:build windows

package diagnose

// loginShellHint explains the most common reason a session never picked up
// sshakku's wiring. PowerShell has no login shell to be or not to be: it reads
// its profile in every session it is not told to skip one for, so the wiring
// is missing here for a different pair of reasons than on a Unix — the session
// was started with -NoProfile, or the execution policy in force refuses to run
// the profile script at all, which it does silently.
const loginShellHint = "likely because this session's PowerShell profile was never read — it was " +
	"started with -NoProfile, or the execution policy refuses to run it (Get-ExecutionPolicy -List " +
	"says which one applies); starting a new session without -NoProfile is what picks the wiring up"
