//go:build windows

package install

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Windows PowerShell 5.x exists on no other system, and it is the edition that
// makes the query hard: it writes a CLIXML progress payload to standard error,
// it renders standard output in the machine's OEM code page unless told
// otherwise, and it answers an encoded command that begins with a byte-order
// mark by printing nothing and exiting zero. Asking PowerShell 7 proves none
// of that — it tolerates all three.
func TestWindowsPowerShellAnswersAboutItself(t *testing.T) {
	exe, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("no Windows PowerShell here to ask")
	}

	host, err := AskHost(t.Context(), exe)

	require.NoError(t, err, "the CLIXML this edition writes to standard error must not reach the parser")
	assert.Equal(t, "Desktop", host.Edition)
	assert.NotEmpty(t, host.Profiles.CurrentUserAllHosts)
	assert.NotEmpty(t, host.Profiles.AllUsersAllHosts)
	assert.Contains(t, host.ExecutionPolicyByScope, "CurrentUser")
}

// Handing the query over as a script file puts it under the execution policy,
// and that is the failure a Windows machine will actually produce: the factory
// default for Windows PowerShell refuses script files outright. This drives a
// real host into a real refusal — process scope outranks the account's own
// setting, so it needs nothing changed on this machine — and checks that the
// refusal is recognised from what PowerShell really prints, rather than from a
// specimen written from memory.
func TestARealPolicyRefusalIsRecognisedFromWhatTheHostPrints(t *testing.T) {
	exe, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("no Windows PowerShell here to refuse")
	}
	script, cleanup, err := queryScriptOnDisk()
	require.NoError(t, err)
	defer cleanup()

	args := append([]string{"-ExecutionPolicy", "Restricted"}, queryArguments(script)...)
	cmd := exec.CommandContext(t.Context(), exe, args...)
	cmd.Env = childEnvironment(os.Environ())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()

	require.Error(t, err, "a policy of Restricted must not run this file")
	require.Empty(t, stdout)
	assert.Contains(t, explain(stderr.String()), "execution policy",
		"the identifier PowerShell prints is what this is recognised by, and it is not translated")
	assert.Contains(t, explain(stderr.String()), "profile",
		"which is the consequence the user is owed")
}

// The two editions are separate targets, and this is the assertion that says
// why: on one machine, one account, they keep their profiles in different
// directories. An install that asked one and assumed the other would wire the
// wrong file and report the right one.
func TestTheTwoEditionsDoNotShareTheirProfiles(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no PowerShell 7 here")
	}
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("no Windows PowerShell here")
	}

	seven, err := AskHost(t.Context(), pwsh)
	require.NoError(t, err)
	five, err := AskHost(t.Context(), powershell)
	require.NoError(t, err)

	assert.NotEqual(t, seven.Profiles.CurrentUserAllHosts, five.Profiles.CurrentUserAllHosts)
	assert.NotEqual(t, seven.Profiles.AllUsersAllHosts, five.Profiles.AllUsersAllHosts)
	assert.Equal(t, "Core", seven.Edition)
	assert.Equal(t, "Desktop", five.Edition)
}
