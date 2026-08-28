//go:build windows

package prompt

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

var (
	// errHostWouldNotStart stands for a real failure to start a process, which
	// the code under test cannot be made to produce on demand.
	errHostWouldNotStart = errors.New("boom")
	// errNotOnPath is what a PATH lookup says about a program that is not there.
	errNotOnPath = errors.New("executable file not found in %PATH%")
)

// hostsPresent builds a PATH lookup that finds exactly the named hosts.
func hostsPresent(names ...string) func(string) (string, error) {
	return func(want string) (string, error) {
		for _, n := range names {
			if n == want {
				return `C:\` + n, nil
			}
		}
		return "", errNotOnPath
	}
}

func TestPowerShellPromptHandsBackWhatWasTyped(t *testing.T) {
	// A passphrase is whatever the user typed, and a trailing space is theirs:
	// the script writes the text with no line ending after it, so nothing here
	// has a newline to strip and nothing may strip anything else either.
	r := runtest.NewRunner().On("pwsh.exe", runtest.Stdout("correct horse ", dialogAnswered))
	got, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	require.NoError(t, err, "a box the user answered must hand the answer back")
	assert.Equal(t, "correct horse ", got,
		"and it must be what they typed, to the last character: a passphrase may end in a space")
}

func TestPowerShellPromptRunsTheScriptWithTheKeyNameAsAnArgument(t *testing.T) {
	r := runtest.NewRunner().On("pwsh.exe", runtest.Stdout("x", dialogAnswered))
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	require.NoError(t, err, "putting the box on the screen must succeed")

	require.Len(t, r.Calls, 1, "a host must actually be run")
	args := r.Calls[0].Args
	require.Len(t, args, 6, "the host is given its switches, the script and the key name")
	assert.Equal(t, []string{"-NoProfile", "-NonInteractive", "-STA"}, args[:3],
		"a window wants a single-threaded apartment, and a profile must have nothing to say on the stream the passphrase travels on")
	assert.Equal(t, "-File", args[3], "the script is run as a file, which is what makes the key name an argument")
	assert.True(t, strings.HasSuffix(args[4], ".ps1"),
		"and the file has to be named the way a host will agree to run one: %s", args[4])
	// The key name is an argument, not part of the script: a name that closed
	// the string it had been pasted into would otherwise carry on as PowerShell.
	assert.Equal(t, "id_ed25519", args[5],
		"the key name travels as an argument: pasted into the script, a name that closed the string it sat in "+
			"would carry on as PowerShell of its own")
}

func TestPowerShellPromptTheScriptIsOnDiskWhileTheBoxIsUp(t *testing.T) {
	var body []byte
	r := runtest.NewRunner().On("pwsh.exe", func(c run.Cmd) (run.Result, error) {
		body, _ = os.ReadFile(c.Args[4])
		return run.Result{Stdout: []byte("x")}, nil
	})
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	require.NoError(t, err, "putting the box on the screen must succeed")
	assert.Equal(t, passphraseDialog, string(body),
		"what the host reads while it runs must be the dialog SSHakku ships, present on disk at that moment")
}

func TestPowerShellPromptTheScriptDoesNotOutliveThePrompt(t *testing.T) {
	var path string
	r := runtest.NewRunner().On("pwsh.exe", func(c run.Cmd) (run.Result, error) {
		path = c.Args[4]
		return run.Result{Stdout: []byte("x")}, nil
	})
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	require.NoError(t, err, "putting the box on the screen must succeed")
	_, err = os.Stat(path)
	assert.ErrorIsf(t, err, os.ErrNotExist, "and the script must not outlive the prompt: %s", path)
}

func TestPowerShellPromptADismissedBoxIsAnAnswer(t *testing.T) {
	r := runtest.NewRunner().On("pwsh.exe", runtest.Stdout("", dialogDismissed))
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, ErrCanceled,
		"closing the box is a decision, and must be passed on as one rather than as a failure")
}

func TestPowerShellPromptAWindowThatNeverDrewIsNotADismissal(t *testing.T) {
	// The case this distinction exists for: an execution policy that will not
	// load the script exits 1 with nothing on screen. Read as a dismissal, one
	// such machine would look like a user closing the box, and the default for
	// a dismissal ends the asking for the rest of the login — so every key
	// would be given up on without anything ever having been shown.
	policy := "File C:\\Users\\u\\sshakku-prompt-1.ps1 cannot be loaded because running scripts is disabled on this system.\r\n" +
		"At line:1 char:1\r\n"
	r := runtest.NewRunner().On("pwsh.exe", func(run.Cmd) (run.Result, error) {
		return run.Result{Stderr: []byte(policy), Code: 1}, nil
	})
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")

	require.ErrorIs(t, err, errDialogNeverDrew,
		"a box that never drew is a failure to ask, and one a caller can match on rather than read")
	assert.NotErrorIs(t, err, ErrCanceled,
		"and must not be reported as the user dismissing it: nothing appeared for them to dismiss")
	assert.Contains(t, err.Error(), "running scripts is disabled on this system",
		"the reason the host gave is what makes this something the user can act on")
	assert.NotContains(t, err.Error(), "At line:1",
		"and it is one line: this goes into a log line, not a stack of them")
}

func TestPowerShellPromptAHostThatExplainedNothingIsStillReported(t *testing.T) {
	// A code nobody recognises and not a word about it is still a box that did
	// not draw. The line has to say so, rather than trail off where the reason
	// would have been and leave a reader wondering what was cut.
	r := runtest.NewRunner().On("pwsh.exe", runtest.Stdout("", 3))
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	require.ErrorIs(t, err, errDialogNeverDrew,
		"an exit code that is neither an answer nor a dismissal is a failure to ask")
	assert.NotErrorIs(t, err, ErrCanceled, "and it is not the user's decision either")
	assert.Contains(t, err.Error(), "said nothing about why",
		"a host that explained nothing is reported as having explained nothing")
}

func TestPowerShellPromptAHostThatWouldNotStartIsAnError(t *testing.T) {
	r := runtest.NewRunner().On("pwsh.exe", runtest.Fails(errHostWouldNotStart))
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, errHostWouldNotStart,
		"a box that could not be started must be reported as that, not as one the user dismissed")
}

func TestPowerShellPromptChoosesTheHostThatIsThere(t *testing.T) {
	t.Run("the faster one when both are installed", func(t *testing.T) {
		r := runtest.NewRunner().
			On("pwsh.exe", runtest.Stdout("x", dialogAnswered)).
			On("powershell.exe", runtest.Stdout("wrong host", dialogAnswered))
		got, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe", "powershell.exe")}.
			Prompt(t.Context(), "id_ed25519")
		require.NoError(t, err, "a machine with both hosts must still be asked in one of them")
		assert.Equal(t, "x", got, "and it must be the one that reaches a window first")
	})

	t.Run("the one every Windows has, when it is the only one", func(t *testing.T) {
		r := runtest.NewRunner().On("powershell.exe", runtest.Stdout("x", dialogAnswered))
		got, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("powershell.exe")}.
			Prompt(t.Context(), "id_ed25519")
		require.NoError(t, err, "a machine with nothing installed still has one host to draw with")
		assert.Equal(t, "x", got, "and it is the one that must be used")
	})

	t.Run("neither, and nothing is run", func(t *testing.T) {
		r := runtest.NewRunner()
		_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent()}.Prompt(t.Context(), "id_ed25519")
		require.ErrorIs(t, err, errNoPowerShellHost, "with no host there is no box, and saying so is the answer")
		assert.Empty(t, r.Calls, "and nothing may be run to find that out")
	})
}

func TestPowerShellPrompterSaysWhetherItCanAsk(t *testing.T) {
	assert.True(t, PowerShellPrompter{lookPath: hostsPresent("powershell.exe")}.Available(t.Context()),
		"a host on PATH is a box that can be drawn")
	assert.False(t, PowerShellPrompter{lookPath: hostsPresent()}.Available(t.Context()),
		"and no host at all is not")
	assert.Equal(t, "powershell", PowerShellPrompter{}.Name(),
		"the name is what gui_prompter calls it, so a message about it names something the user can write")
	assert.Contains(t, Unavailable(PowerShellPrompter{}), "PowerShell host",
		"and the reason names what is missing rather than the dialog: either host would do")
}

func TestPowerShellPrompterAsksThisSystemsOwnPathWhenNobodyInjectedOne(t *testing.T) {
	// Every other test here hands the prompter a lookup of its own, so the one
	// it uses in production — the real PATH — would go unexercised. A Windows
	// that can run this suite has a PowerShell host: the host is part of the
	// system rather than something installed, so the default seam finding none
	// is the seam being broken, not a machine without one.
	assert.True(t, PowerShellPrompter{}.Available(t.Context()),
		"with no lookup injected the prompter must ask the real PATH, where this system keeps its own host")
}

func TestPowerShellPromptAScriptThatCouldNotBeWrittenOutIsReported(t *testing.T) {
	// Distinct from the file that could not be created: this one exists and the
	// writing fails, which is the branch that has to take it back off the disk
	// again rather than leave an empty script for a host to run.
	f, err := os.CreateTemp(t.TempDir(), "closed-*.ps1")
	require.NoError(t, err, "the fixture needs a file to close")
	require.NoError(t, f.Close(), "and it has to be closed for the write to fail")

	original := createDialogScript
	t.Cleanup(func() { createDialogScript = original })
	createDialogScript = func() (*os.File, error) { return f, nil }

	r := runtest.NewRunner()
	_, err = PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, os.ErrClosed, "a script that could not be written out is a box that cannot be drawn")
	assert.Empty(t, r.Calls, "and no host is run against a script that is not there")
	assert.NoFileExists(t, f.Name(), "the half-written file does not stay behind for anyone to run")
}

func TestPowerShellPromptAScriptThatCannotBeWrittenIsReported(t *testing.T) {
	original := createDialogScript
	t.Cleanup(func() { createDialogScript = original })
	createDialogScript = func() (*os.File, error) { return nil, errHostWouldNotStart }

	r := runtest.NewRunner()
	_, err := PowerShellPrompter{Runner: r, lookPath: hostsPresent("pwsh.exe")}.Prompt(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, errHostWouldNotStart, "a script that could not be laid down is a box that cannot be drawn")
	assert.Empty(t, r.Calls, "and no host is run with no script to run")
}
