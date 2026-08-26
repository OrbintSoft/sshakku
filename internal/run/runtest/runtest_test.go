package runtest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errExecutableFileNotFoundIn = errors.New("executable file not found in $PATH")
	errKeepassxcCliVanished     = errors.New("keepassxc-cli vanished")
)

// The stand-ins here are what every other package's tests read their answers
// from, so one that answers the wrong call, or quietly answers a call nobody
// registered, does not fail — it makes the tests above it agree with something
// that never happened. That is what these tests are for.

func TestRunnerAnswersPerProgram(t *testing.T) {
	r := NewRunner().
		On("ssh-add", Stdout("the agent's answer", 0)).
		On("secret-tool", Stdout("the wallet's answer", 1))

	added, err := r.Run(t.Context(), run.Cmd{Name: "ssh-add", Args: []string{"-l"}})
	require.NoError(t, err, "a registered program must answer")
	assert.Equal(t, "the agent's answer", string(added.Stdout),
		"and each program has to answer for itself, or a test stubbing several tools reads one's answer as another's")
	assert.Zero(t, added.Code, "the exit code registered for it is the one it must report")

	wallet, err := r.Run(t.Context(), run.Cmd{Name: "secret-tool"})
	require.NoError(t, err, "a non-zero exit is an answer, not a failure to run")
	assert.Equal(t, 1, wallet.Code, "and the code registered for it must arrive intact")
}

func TestRunnerRefusesAProgramNobodyRegistered(t *testing.T) {
	r := NewRunner().On("ssh-add", Stdout("", 0))

	_, err := r.Run(t.Context(), run.Cmd{Name: "ssh-keygen"})
	require.Error(t, err,
		"a component that ran something the test never stubbed has to say so; answering a zero Result "+
			"would let it read 'no fingerprint' as a fact and the test would agree with it")
	assert.Contains(t, err.Error(), "ssh-keygen", "and the error has to name what was run, or there is nothing to go on")
}

func TestRunnerRecordsEveryCall(t *testing.T) {
	r := NewRunner().On("secret-tool", Stdout("", 0))

	_, err := r.Run(t.Context(), run.Cmd{Name: "secret-tool", Args: []string{"store"}, Stdin: "hunter2"})
	require.NoError(t, err, "the stubbed program must answer")

	require.Len(t, r.Calls, 1, "what crossed the process boundary is what a test asserts on")
	assert.Equal(t, []string{"store"}, r.Calls[0].Args, "the argv has to be kept as it was sent")
	assert.Equal(t, "hunter2", r.Calls[0].Stdin,
		"and so does the standard input, which is where a passphrase travels to keep it out of argv")
}

func TestFailsReportsAProcessThatNeverStarted(t *testing.T) {
	boom := errExecutableFileNotFoundIn
	r := NewRunner().On("zenity", Fails(boom))

	_, err := r.Run(t.Context(), run.Cmd{Name: "zenity"})
	require.ErrorIs(t, err, boom,
		"a tool that is not installed is the one outcome a Runner reports as an error, and a stand-in "+
			"for it has to be able to say so rather than only exit non-zero")
}

func TestRecorderAnswersInCallOrder(t *testing.T) {
	r := &Recorder{Results: []run.Result{{Code: 1}, {Code: 0, Stdout: []byte("second")}}}

	first, err := r.Run(t.Context(), run.Cmd{Name: "keepassxc-cli", Args: []string{"show"}})
	require.NoError(t, err, "a scripted call must answer")
	assert.Equal(t, 1, first.Code, "the first answer belongs to the first call")

	second, err := r.Run(t.Context(), run.Cmd{Name: "keepassxc-cli", Args: []string{"add"}})
	require.NoError(t, err, "and so must the next")
	assert.Equal(t, "second", string(second.Stdout),
		"a component whose subject is a sequence of calls is only tested if each one gets its own answer")

	require.Len(t, r.Calls, 2, "and every call has to be kept")
	assert.Equal(t, []string{"add"}, r.Calls[1].Args, "in the order they were made")
}

func TestRecorderErrsTakePrecedenceAtTheSameIndex(t *testing.T) {
	boom := errKeepassxcCliVanished
	r := &Recorder{
		Results: []run.Result{{Code: 0}, {Code: 0}},
		Errs:    []error{nil, boom},
	}

	_, err := r.Run(t.Context(), run.Cmd{Name: "keepassxc-cli"})
	require.NoError(t, err, "a nil at that index leaves the scripted Result standing")

	_, err = r.Run(t.Context(), run.Cmd{Name: "keepassxc-cli"})
	require.ErrorIs(t, err, boom,
		"a test about the step that could not start needs to fail that one step and no other, "+
			"which is what an error placed at its index is for")
}

func TestRecorderRunsOutOfScriptQuietly(t *testing.T) {
	r := &Recorder{}

	res, err := r.Run(t.Context(), run.Cmd{Name: "keepassxc-cli"})
	require.NoError(t, err, "a call past the end of the script is not a failure to run")
	assert.Zero(t, res.Code,
		"it succeeds with nothing to say, so a test scripting only the calls it cares about "+
			"is not made to script the ones it does not")
}
