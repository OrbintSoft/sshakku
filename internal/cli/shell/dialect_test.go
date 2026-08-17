package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialect resolves a dialect by name for a test that needs one to render with.
func dialect(t *testing.T, name string) Dialect {
	t.Helper()
	d, err := named(name)
	require.NoErrorf(t, err, "%s is a dialect this program has", name)
	return d
}

// TestEachDialectPrintsWhatItsShellReads verifies F43: the lines printed for a
// shell to run are in a language that shell speaks — a Bourne assignment for
// posix, a PowerShell one for powershell, each with the environment form the
// same shell would need.
func TestEachDialectPrintsWhatItsShellReads(t *testing.T) {
	posix, powershell := dialect(t, Posix), dialect(t, PowerShell)

	assert.Equal(t, "agent_sock='/run/user/1000/sshakku/agent.sock'\n",
		posix.SetVar("agent_sock", "/run/user/1000/sshakku/agent.sock"),
		"what bash reads as setting a variable")
	assert.Equal(t, "export SSH_ASKPASS='/usr/local/bin/sshakku-askpass'\n",
		posix.SetEnv("SSH_ASKPASS", "/usr/local/bin/sshakku-askpass"),
		"what bash reads as setting one in the environment")
	assert.Equal(t, "$agent_sock = '\\\\.\\pipe\\sshakku'\n",
		powershell.SetVar("agent_sock", `\\.\pipe\sshakku`),
		"what PowerShell reads as setting a variable")
	assert.Equal(t, "$env:SSH_ASKPASS = 'C:\\Program Files\\SSHakku\\sshakku-askpass.exe'\n",
		powershell.SetEnv("SSH_ASKPASS", `C:\Program Files\SSHakku\sshakku-askpass.exe`),
		"what PowerShell reads as setting one in the environment")
}

// TestAValueSurvivesItsOwnShellsQuoting verifies the half of F43 that says
// whatever is in a path arrives whole, including the characters the shell would
// otherwise read as the end of the quoted value.
//
// PowerShell's five: it ends a literal opened with an apostrophe at any of the
// four curly quotes too, so escaping only U+0027 leaves `C:\Users\O’Brien` a
// parse error rather than a path — and that is an account name, not a curiosity.
func TestAValueSurvivesItsOwnShellsQuoting(t *testing.T) {
	posix, powershell := dialect(t, Posix), dialect(t, PowerShell)

	assert.Equal(t, `p='/home/o'\''brien/.ssh'`+"\n",
		posix.SetVar("p", "/home/o'brien/.ssh"),
		"a Bourne literal is left and re-entered around the apostrophe")

	for _, quote := range []string{"'", "\u2018", "\u2019", "\u201A", "\u201B"} {
		t.Run(quote, func(t *testing.T) {
			assert.Equal(t, "$p = 'C:\\Users\\O"+quote+quote+"Brien'\n",
				powershell.SetVar("p", `C:\Users\O`+quote+`Brien`),
				"every character PowerShell reads as a quote is doubled, or the literal ends there")
		})
	}
}

// A caller that is placing a value somewhere else — into a hook this program
// renders, where the assignment is already written and only the value is being
// supplied — gets the same quoting the assignments above use, and not a rule of
// its own. That is the whole point of it being reachable: one shell, one answer
// about what a literal is, wherever the literal is going.
func TestAValueCanBeQuotedForItsShellWithoutAnAssignmentAroundIt(t *testing.T) {
	posix, powershell := dialect(t, Posix), dialect(t, PowerShell)

	assert.Equal(t, `'/home/o'\''brien/bin/sshakku'`, posix.Quote("/home/o'brien/bin/sshakku"))
	assert.Equal(t, `'C:\Users\O''Brien\sshakku.exe'`, powershell.Quote(`C:\Users\O'Brien\sshakku.exe`))

	// The same value, quoted once by each way in: whatever a caller is building,
	// what lands in the file is what the shell would have been given anyway.
	assert.Equal(t, "p="+posix.Quote("/tmp/x")+"\n", posix.SetVar("p", "/tmp/x"))
}

// Code that has already worked out which language it needs — a renderer writing
// a file for a shell it has just identified — asks by name, and is refused by
// name for one this program cannot print rather than handed another language.
func TestADialectCanBeAskedForByNameWithoutAFlagToRead(t *testing.T) {
	for _, name := range []string{Posix, PowerShell} {
		got, err := Named(name)
		require.NoErrorf(t, err, "%s is a dialect this program has", name)
		assert.Equal(t, name, got.name)
	}

	_, err := Named("fish")
	require.Error(t, err, "a language this program cannot print is never quietly answered in another")
	assert.Contains(t, err.Error(), "fish")
}

// TestTheDialectIsTheOneAskedForOrPosix verifies F43's other half: the dialect
// is what --shell says and nothing else decides it, no flag means posix, and a
// dialect this program has not got is refused by name rather than answered in
// the wrong language.
func TestTheDialectIsTheOneAskedForOrPosix(t *testing.T) {
	answered := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", nil, Posix},
		{"posix by name", []string{"--shell=posix"}, Posix},
		{"posix as a separate word", []string{"--shell", "posix"}, Posix},
		{"powershell by name", []string{"--shell=powershell"}, PowerShell},
		{"powershell as a separate word", []string{"--shell", "powershell"}, PowerShell},
	}
	for _, tc := range answered {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromArgs(tc.args)
			require.NoErrorf(t, err, "%q names a dialect this program has", tc.args)
			assert.Equal(t, tc.want, got.name, "the dialect the caller asked for")
		})
	}

	refused := []struct {
		name string
		args []string
	}{
		{"a shell this program does not speak", []string{"--shell=fish"}},
		{"a dialect spelled as nothing", []string{"--shell="}},
		{"the flag with no value after it", []string{"--shell"}},
		{"an argument that is not the flag", []string{"--dialect=powershell"}},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromArgs(tc.args)
			assert.Errorf(t, err, "%q must be refused, not answered in some other language", tc.args)
		})
	}

	_, err := FromArgs([]string{"--shell=fish"})
	require.Error(t, err, "fish is not a dialect this program has")
	assert.Contains(t, err.Error(), "fish", "the refusal names what was asked for")
	assert.Contains(t, err.Error(), PowerShell, "and what could have been asked for instead")
}
