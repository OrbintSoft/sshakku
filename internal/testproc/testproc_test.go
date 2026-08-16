package testproc

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	Serve()
	os.Exit(m.Run())
}

// The claim this package makes is that a test can be handed a real process
// that does what the case needs. Everything else here checks act in isolation;
// only this checks that the child is really reached, really does it, and really
// carries the exit code back.
func TestTheChildIsThisBinaryDoingWhatItWasAsked(t *testing.T) {
	name, args := Command(t, Emit, "out", "err", "3")

	cmd := exec.CommandContext(t.Context(), name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, "a child that exited non-zero reports it as an exit, not as a failure to start")
	assert.Equal(t, 3, exit.ExitCode())
	assert.Equal(t, "out", stdout.String())
	assert.Equal(t, "err", stderr.String())
}

func TestTheChildIsNamedAsThisBinaryAndMarkedAsAChild(t *testing.T) {
	name, args := Command(t, EchoStdin, "extra")

	exe, err := os.Executable()
	require.NoError(t, err)
	assert.Equal(t, exe, name, "the program is this test binary, which is what makes it available everywhere")
	assert.Equal(t, []string{marker, EchoStdin, "extra"}, args)
}

// An ordinary test run must be left alone: Serve is called from TestMain in
// every package that uses this, and a stray match would swallow the suite.
func TestAnOrdinaryRunIsNotMistakenForAChild(t *testing.T) {
	require.NotEmpty(t, os.Args)
	assert.NotEqual(t, marker, os.Args[0], "the marker is a first argument, never a program name")

	if len(os.Args) > 1 {
		assert.NotEqual(t, marker, os.Args[1],
			"this test is running, so Serve returned, so the marker was not there")
	}
}

func TestEmitWritesToBothStreamsAndCarriesTheCode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := act([]string{Emit, "to out", "to err", "7"}, nil, &stdout, &stderr)

	assert.Equal(t, 7, code)
	assert.Equal(t, "to out", stdout.String())
	assert.Equal(t, "to err", stderr.String())
}

func TestSleepWaitsAndSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer

	start := time.Now()
	code := act([]string{Sleep, "50ms"}, nil, &stdout, &stderr)

	assert.Zero(t, code)
	assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
	assert.Empty(t, stderr.String())
}

func TestEchoStdinGivesBackWhatItWasHanded(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := act([]string{EchoStdin}, strings.NewReader("hunter2\n"), &stdout, &stderr)

	assert.Zero(t, code)
	assert.Equal(t, "hunter2\n", stdout.String())
}

func TestEchoStdinReportsAnInputItCouldNotRead(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := act([]string{EchoStdin}, failingReader{}, &stdout, &stderr)

	assert.Equal(t, 1, code, "a child that could not do its job must not exit as though it had")
	assert.Contains(t, stderr.String(), "standard input")
}

func TestEchoEnvAnswersOnePerLineIncludingTheOnesThatAreNotSet(t *testing.T) {
	t.Setenv("SSHAKKU_TESTPROC_PRESENT", "here")
	var stdout, stderr bytes.Buffer

	code := act([]string{EchoEnv, "SSHAKKU_TESTPROC_PRESENT", "SSHAKKU_TESTPROC_ABSENT"}, nil, &stdout, &stderr)

	assert.Zero(t, code)
	assert.Equal(t, []string{"here", "", ""}, strings.Split(stdout.String(), "\n"),
		"an unset variable answers with an empty line, so the caller still counts one answer per name")
}

// A child asked for something it cannot do must fail loudly. Exiting 0 in
// silence would let a broken test read the emptiness as the program's answer.
func TestAChildAskedForNonsenseSaysSoAndFails(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		says string
	}{
		{"no mode at all", nil, "no mode given"},
		{"a mode nobody serves", []string{"dance"}, `no such mode "dance"`},
		{"emit without its three arguments", []string{Emit, "only one"}, "takes"},
		{"emit with an exit code that is not a number", []string{Emit, "", "", "soon"}, "takes"},
		{"sleep without a duration", []string{Sleep}, "takes"},
		{"sleep for something that is not a duration", []string{Sleep, "a while"}, "takes"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := act(c.argv, nil, &stdout, &stderr)

			assert.Equal(t, 2, code)
			assert.Contains(t, stderr.String(), c.says)
			assert.Empty(t, stdout.String(), "nothing may reach standard output, where a caller reads answers")
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("this input is broken") }
