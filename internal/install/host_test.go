package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What the two editions actually answer, with the account renamed. Kept as
// their own answers rather than as one specimen, because the whole reason each
// host is asked separately is that these two differ: different profile
// directories, and a different execution policy for the same account.
const (
	powerShell7Answer = `{"edition":"Core","version":"7.6.4","psHome":"C:\\Program Files\\PowerShell\\7",` +
		`"languageMode":"FullLanguage","effectiveExecutionPolicy":"Bypass",` +
		`"executionPolicyByScope":{"MachinePolicy":"Undefined","UserPolicy":"Undefined","Process":"Undefined",` +
		`"CurrentUser":"Bypass","LocalMachine":"RemoteSigned"},` +
		`"profiles":{"default":"C:\\Users\\example\\Documents\\PowerShell\\Microsoft.PowerShell_profile.ps1",` +
		`"currentUserAllHosts":"C:\\Users\\example\\Documents\\PowerShell\\profile.ps1",` +
		`"currentUserCurrentHost":"C:\\Users\\example\\Documents\\PowerShell\\Microsoft.PowerShell_profile.ps1",` +
		`"allUsersAllHosts":"C:\\Program Files\\PowerShell\\7\\profile.ps1",` +
		`"allUsersCurrentHost":"C:\\Program Files\\PowerShell\\7\\Microsoft.PowerShell_profile.ps1"}}`

	windowsPowerShellAnswer = `{"edition":"Desktop","version":"5.1.26100.9168",` +
		`"psHome":"C:\\Windows\\System32\\WindowsPowerShell\\v1.0",` +
		`"languageMode":"FullLanguage","effectiveExecutionPolicy":"RemoteSigned",` +
		`"executionPolicyByScope":{"MachinePolicy":"Undefined","UserPolicy":"Undefined","Process":"Undefined",` +
		`"CurrentUser":"RemoteSigned","LocalMachine":"Unrestricted"},` +
		`"profiles":{"default":"C:\\Users\\example\\Documents\\WindowsPowerShell\\Microsoft.PowerShell_profile.ps1",` +
		`"currentUserAllHosts":"C:\\Users\\example\\Documents\\WindowsPowerShell\\profile.ps1",` +
		`"currentUserCurrentHost":"C:\\Users\\example\\Documents\\WindowsPowerShell\\Microsoft.PowerShell_profile.ps1",` +
		`"allUsersAllHosts":"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\profile.ps1",` +
		`"allUsersCurrentHost":"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\Microsoft.PowerShell_profile.ps1"}}`
)

func TestEachEditionIsReadAsItsOwnAnswer(t *testing.T) {
	seven, err := parseHost([]byte(powerShell7Answer))
	require.NoError(t, err)
	five, err := parseHost([]byte(windowsPowerShellAnswer))
	require.NoError(t, err)

	assert.Equal(t, "Core", seven.Edition)
	assert.Equal(t, "Desktop", five.Edition)
	assert.NotEqual(t, seven.Profiles.CurrentUserAllHosts, five.Profiles.CurrentUserAllHosts,
		"the two editions keep their profiles in different directories, which is why both are asked")
	assert.NotEqual(t, seven.ExecutionPolicyByScope["CurrentUser"], five.ExecutionPolicyByScope["CurrentUser"],
		"the same account can be governed differently in each, from separate registry keys")
	assert.Equal(t, "FullLanguage", seven.LanguageMode)
	assert.Equal(t, "C:\\Program Files\\PowerShell\\7\\profile.ps1", seven.Profiles.AllUsersAllHosts)
}

// A host that was handed something it could not run exits successfully having
// printed nothing. Reading that as a machine whose profiles are all at the
// empty path would wire a hook into nowhere and report success.
func TestAHostThatSaidNothingIsAFailure(t *testing.T) {
	_, err := parseHost(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "printed nothing")

	_, err = parseHost([]byte("   \r\n  "))
	require.Error(t, err, "whitespace is nothing")
}

func TestAnAnswerThatIsNotTheAnswerIsAFailure(t *testing.T) {
	_, err := parseHost([]byte("Get-ExecutionPolicy : the term is not recognised"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not answer with JSON")

	_, err = parseHost([]byte(`{"edition":"Core","profiles":{}}`))
	require.Error(t, err, "JSON that names no profile answers nothing this program can use")
	assert.Contains(t, err.Error(), "named none of its profiles")
}

// What a host is asked to run is legible to whoever looks: the person typing
// the install, an administrator reading a process list, and the endpoint
// protection deciding whether to allow it. A script handed over as base64 is
// none of those things, and it is also the one shape that can never carry a
// signature.
func TestTheQueryIsHandedOverInTheClear(t *testing.T) {
	args := queryArguments(`C:\Users\example\AppData\Local\Temp\q\query-host.ps1`)

	assert.Equal(t, []string{
		"-NoProfile", "-NonInteractive",
		"-File", `C:\Users\example\AppData\Local\Temp\q\query-host.ps1`,
	}, args)
}

// The mark stays. Windows PowerShell reads a script file without one in the
// machine's ANSI code page, which corrupts exactly the non-ASCII paths this
// query exists to report.
func TestTheScriptIsWrittenOutExactlyAsItIsInTheTree(t *testing.T) {
	require.NotEmpty(t, queryHostScript)
	require.True(t, hasBOM(queryHostScript), "PSScriptAnalyzer asks every .ps1 here to carry one")

	path, cleanup, err := queryScriptOnDisk()
	require.NoError(t, err)
	defer cleanup()

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, queryHostScript, written)
	assert.Equal(t, ".ps1", filepath.Ext(path),
		"an interpreter decides whether it will run a file by its extension")
	assert.Contains(t, string(written), "ConvertTo-Json")
}

func TestTheScriptIsTakenBackAfterwards(t *testing.T) {
	path, cleanup, err := queryScriptOnDisk()
	require.NoError(t, err)
	cleanup()

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "a query leaves nothing behind on the disk it borrowed")
}

// A policy that forbids script files forbids the profile too, so the hook this
// program would write there could never run either. Saying that is the whole
// value of telling this failure apart from the rest.
func TestAPolicyRefusalIsToldApartFromABrokenQuery(t *testing.T) {
	refused := explain(`& : File C:\q\query-host.ps1 cannot be loaded because running scripts is disabled
    + CategoryInfo          : SecurityError: (:) [], PSSecurityException
    + FullyQualifiedErrorId : UnauthorizedAccess`)

	assert.Contains(t, refused, "execution policy")
	assert.Contains(t, refused, "profile",
		"the consequence a user needs is that the hook would not run either")
	// The remedy is the half a person can act on, and this is the only path
	// that reaches them for a policy that forbids script files: the host that
	// would have reported its own policy cannot run the query that asks.
	assert.Contains(t, refused, "Set-ExecutionPolicy",
		"and what it would take to change it, since a consequence with no remedy leaves them stuck")

	other := explain("Get-ExecutionPolicy : the term is not recognised")
	assert.NotContains(t, other, "execution policy",
		"a different failure must not be dressed up as a policy refusal")
	assert.Contains(t, other, "not recognised", "but it must still repeat what the host said")

	assert.Empty(t, explain("   \n "), "a host that said nothing adds nothing to the message")
}

// Both of these describe the PowerShell that started this program. Handing
// either to the PowerShell this program starts makes it answer about the wrong
// session — or, for the module path, stop being able to answer at all.
func TestTheHostIsNotToldAboutTheShellThatStartedUs(t *testing.T) {
	kept := childEnvironment([]string{
		"PATH=/usr/bin",
		"PSModulePath=C:\\Program Files\\PowerShell\\7\\Modules",
		"HOME=/home/example",
		"PSExecutionPolicyPreference=Bypass",
	})

	assert.Equal(t, []string{"PATH=/usr/bin", "HOME=/home/example"}, kept)
}

func TestTheseNamesAreMatchedTheWayTheSystemMatchesThem(t *testing.T) {
	kept := childEnvironment([]string{"psmodulepath=x", "PSMODULEPATH=y", "PsModulePath=z", "KEEP=1"})

	assert.Equal(t, []string{"KEEP=1"}, kept,
		"environment variable names are case-insensitive where this runs, so matching one spelling would let the others through")
}

func TestAnEntryWithNoNameIsLeftAlone(t *testing.T) {
	kept := childEnvironment([]string{"NOEQUALSSIGN", "KEEP=1"})

	assert.Equal(t, []string{"NOEQUALSSIGN", "KEEP=1"}, kept,
		"whatever that is, it is not one of the two being dropped, and inventing a rule for it is not this function's job")
}

func TestAskingSomethingThatIsNotThereNamesIt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-powershell")

	_, err := AskHost(t.Context(), missing)

	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
}

// The seam this package exists for: a real PowerShell, handed a real encoded
// payload, answering about itself. Everything above tests what is done with
// the answer; only this tests that the question can be asked at all.
func TestARealPowerShellAnswersAboutItself(t *testing.T) {
	exe, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no pwsh here to ask")
	}

	host, err := AskHost(t.Context(), exe)

	require.NoError(t, err)
	assert.Equal(t, "Core", host.Edition, "pwsh is PowerShell 6 or later, whatever the version")
	assert.NotEmpty(t, host.Version)
	assert.NotEmpty(t, host.Profiles.CurrentUserAllHosts)
	assert.NotEmpty(t, host.Profiles.CurrentUserCurrentHost)
	assert.NotEmpty(t, host.Profiles.AllUsersAllHosts)
	assert.NotEmpty(t, host.LanguageMode)
	assert.Contains(t, host.ExecutionPolicyByScope, "CurrentUser",
		"the scope an install writes for is the one it has to be able to report")
}

func hasBOM(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF
}
