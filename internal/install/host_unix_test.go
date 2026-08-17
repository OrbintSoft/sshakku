//go:build unix

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A host that answered, and named the profiles of the account it is running as
// and none of the machine's — which is what a locked-down installation looks
// like from here. The wiring has to tell that from a host that said nothing at
// all: this one answered the question, and the answer is that there is no such
// file to wire.
const powerShellAnswerWithoutMachineProfiles = `{"edition":"Core","version":"7.6.4",` +
	`"languageMode":"FullLanguage","effectiveExecutionPolicy":"Bypass",` +
	`"profiles":{"default":"C:\\Users\\example\\Documents\\PowerShell\\Microsoft.PowerShell_profile.ps1",` +
	`"currentUserAllHosts":"C:\\Users\\example\\Documents\\PowerShell\\profile.ps1"}}`

// fakePowerShell puts a stand-in for a PowerShell host in a directory of the
// test's own, under the name an install recognises one by, and points it at the
// answer to give.
//
// This system has no PowerShell to ask, and what is being exercised is not the
// asking: it is which file an install chooses out of the five a host reports, and
// that decision has to hold on the machine where the host is real. Where the
// asking itself is the subject, a real interpreter is used instead — see
// TestARealPowerShellAnswersAboutItself, and the Windows tests beside it.
func fakePowerShell(t *testing.T, answer string) string {
	t.Helper()

	fixture, err := os.ReadFile(filepath.Join("testdata", "fake-powershell-host.sh"))
	require.NoError(t, err)

	dir := t.TempDir()
	exe := filepath.Join(dir, "pwsh")
	require.NoError(t, os.WriteFile(exe, fixture, 0o755))

	said := filepath.Join(dir, "answer.json")
	require.NoError(t, os.WriteFile(said, []byte(answer), 0o600))
	t.Setenv("SSHAKKU_TEST_HOST_ANSWER", said)
	return exe
}

// The profile an install wires is the one the host named, and the sweep is every
// profile it named — not a path assembled here from what a PowerShell usually
// keeps where.
func TestTheProfileWiredIsTheOneTheHostNamed(t *testing.T) {
	exe := fakePowerShell(t, powerShell7Answer)

	p, err := resolve(t.Context(), Request{Shell: Auto, ShellExe: exe, Scope: User, Hosts: AllHosts}, Ancestry{})

	require.NoError(t, err)
	assert.Equal(t, PowerShellCore, p.kind, "the name pwsh is this system's answer for which kind that is")
	assert.Equal(t, `C:\Users\example\Documents\PowerShell\profile.ps1`, p.placement.Path)
	assert.False(t, p.placement.DropIn, "nothing in that profile loads a drop-in directory")
	assert.Len(t, p.sweep, 4, "an uninstall looks in every profile the host named, deduplicated")
}

// A host that named no profile for the combination asked has not answered it.
// Writing to the empty path would create a file called "" in whatever directory
// the install was run from and report it as the wiring.
func TestAHostThatNamedNoProfileForWhatWasAskedIsRefused(t *testing.T) {
	exe := fakePowerShell(t, powerShellAnswerWithoutMachineProfiles)

	_, err := resolve(t.Context(), Request{Shell: Auto, ShellExe: exe, Scope: Machine, Hosts: CurrentHost}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), string(CurrentHost), "the refusal says which combination went unanswered")
	assert.Contains(t, err.Error(), string(Machine))
}

// An interpreter that answers to the name and cannot say what it is, is reported
// as that rather than wired on the strength of its name.
func TestAPowerShellThatSaidNothingIsReportedAndNotWired(t *testing.T) {
	exe := fakePowerShell(t, "")

	_, err := resolve(t.Context(), Request{Shell: Auto, ShellExe: exe, Scope: User, Hosts: AllHosts}, Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), exe, "which of the machine's interpreters failed is the whole of the report")
}

// A shell that ran and said nothing is a failure of this mechanism, not a shell
// whose home directory is the empty string: wiring that would name a startup file
// in whatever directory the install was run from.
func TestAShellThatRanAndSaidNothingIsAFailure(t *testing.T) {
	sayingNothing := fakePowerShell(t, "")

	_, err := AskShell(t.Context(), sayingNothing)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "printed nothing")
}
