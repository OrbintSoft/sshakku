package sshtools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errNotOnPath is what a lookup answers with for a name the session has no
// program under, which is the shape of the answer and not its wording.
var errNotOnPath = errors.New("not on this session's PATH")

// aDirectoryHolding makes a directory containing the named files and returns
// it. The files are real, because what decides here is whether a runtime sits
// beside a program, and that is a question the filesystem answers rather than
// one a stand-in can be asked.
func aDirectoryHolding(t *testing.T, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600), name)
	}
	return dir
}

// answering puts a session's PATH in front of the deciding: a name in answers
// resolves to the program it maps to, and every other name is not there. It
// reports how many lookups were made, so a test can also say that none was.
func answering(t *testing.T, answers map[string]string) *int {
	t.Helper()

	old := lookPath
	t.Cleanup(func() { lookPath = old })

	lookups := 0
	lookPath = func(name string) (string, error) {
		lookups++
		if found, ok := answers[name]; ok {
			return found, nil
		}
		return "", errNotOnPath
	}
	return &lookups
}

// itself maps each program to the answer a lookup of its own full path gives,
// which is what looking in a named directory comes to.
func itself(programs ...string) map[string]string {
	answers := make(map[string]string, len(programs))
	for _, program := range programs {
		answers[program] = program
	}
	return answers
}

// TestASystemWithOneOpenSSHRunsWhatTheSessionFinds. Where nothing emulates
// another system there is nothing to choose between, so the name is left for
// the session's own lookup to resolve — the user goes on running the program
// their PATH says they run, rather than one picked for them out of sight.
func TestASystemWithOneOpenSSHRunsWhatTheSessionFinds(t *testing.T) {
	lookups := answering(t, nil)

	tool, err := System{}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, "ssh-add", tool, "the name the caller asked for is the answer")
	assert.Zero(t, *lookups, "nothing had to be looked up to say so")
}

// TestTheSessionsOwnAnswerStandsWhenItCanReachTheAgent. Choosing is for the one
// case that needs it. A program that is not built on an emulation layer talks
// to this system's agent, whoever installed it and wherever it sits.
func TestTheSessionsOwnAnswerStandsWhenItCanReachTheAgent(t *testing.T) {
	onPath := aDirectoryHolding(t, "ssh-add")
	answering(t, map[string]string{"ssh-add": filepath.Join(onPath, "ssh-add")})

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{aDirectoryHolding(t, "ssh-add")},
	}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, "ssh-add", tool, "there was nothing wrong with the session's own answer")
}

// TestABuildForAnEmulationLayerIsPassedOverForOneThatCanReachTheAgent is what
// this package exists for. A shell that emulates POSIX puts its own OpenSSH
// first, and that build speaks to its layer's socket rather than to this
// system's agent: handed a key it answers as though the passphrase were wrong,
// so a correct one is asked for again, and again, and the key is then given up
// on. Nothing about that looks like the wrong program having been run.
func TestABuildForAnEmulationLayerIsPassedOverForOneThatCanReachTheAgent(t *testing.T) {
	emulated := aDirectoryHolding(t, "ssh-add", "msys-2.0.dll")
	native := aDirectoryHolding(t, "ssh-add")
	answers := itself(filepath.Join(native, "ssh-add"))
	answers["ssh-add"] = filepath.Join(emulated, "ssh-add")
	answering(t, answers)

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll", "cygwin1.dll"},
		NativeDirs:        []string{native},
	}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(native, "ssh-add"), tool,
		"the build that can reach this system's agent is the one to run")
}

// TestEveryRuntimeThisSystemNamesIsOneToPassOver: the runtimes are a list
// because there is more than one way to emulate POSIX here, and a caller that
// checked only the first would hand the second's build a passphrase.
func TestEveryRuntimeThisSystemNamesIsOneToPassOver(t *testing.T) {
	for _, runtime := range []string{"msys-2.0.dll", "cygwin1.dll"} {
		t.Run(runtime, func(t *testing.T) {
			emulated := aDirectoryHolding(t, "ssh-add", runtime)
			native := aDirectoryHolding(t, "ssh-add")
			answers := itself(filepath.Join(native, "ssh-add"))
			answers["ssh-add"] = filepath.Join(emulated, "ssh-add")
			answering(t, answers)

			tool, err := System{
				EmulationRuntimes: []string{"msys-2.0.dll", "cygwin1.dll"},
				NativeDirs:        []string{native},
			}.Tool("ssh-add")

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(native, "ssh-add"), tool)
		})
	}
}

// TestASessionWithNothingThatCanReachTheAgentIsToldSo. There is no third
// answer: running the emulated build anyway is what produced the silent
// failure, and returning the name would be the same thing said differently. The
// caller can log this and tell the user; it cannot load a key either way.
func TestASessionWithNothingThatCanReachTheAgentIsToldSo(t *testing.T) {
	emulated := aDirectoryHolding(t, "ssh-add", "msys-2.0.dll")
	answering(t, map[string]string{"ssh-add": filepath.Join(emulated, "ssh-add")})

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{filepath.Join(t.TempDir(), "nothing-was-installed-here")},
	}.Tool("ssh-add")

	require.Error(t, err)
	assert.Empty(t, tool, "a name that cannot reach the agent is not an answer to fall back on")
	assert.Contains(t, err.Error(), filepath.Join(emulated, "ssh-add"),
		"the user has to be told which program was found, or they cannot go and look at it")
	assert.Contains(t, err.Error(), "msys-2.0.dll",
		"and why it was passed over, or the message is an accusation with no evidence")
}

// TestASystemsOwnBuildIsFoundWhereTheSessionWasNeverToldOfIt. A session may
// have been handed a PATH with no OpenSSH on it at all — a shell started by
// something that built its environment from nothing. The system still has its
// own, and it is still the one that reaches the agent.
func TestASystemsOwnBuildIsFoundWhereTheSessionWasNeverToldOfIt(t *testing.T) {
	native := aDirectoryHolding(t, "ssh-add")
	answering(t, itself(filepath.Join(native, "ssh-add")))

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{native},
	}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(native, "ssh-add"), tool)
}

// TestASystemWithNoOpenSSHAtAllReportsTheLookupsOwnAnswer: nothing here can
// improve on "there is no such program", and dressing it up as a refusal would
// send the user looking for a build to remove that was never installed.
func TestASystemWithNoOpenSSHAtAllReportsTheLookupsOwnAnswer(t *testing.T) {
	answering(t, nil)

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{filepath.Join(t.TempDir(), "nothing-was-installed-here")},
	}.Tool("ssh-add")

	require.ErrorIs(t, err, errNotOnPath)
	assert.Empty(t, tool)
}

// TestTheFirstOfThisSystemsOwnDirectoriesThatHasItWins, so a system that keeps
// more than one place for its own OpenSSH states its order of preference in the
// table rather than leaving it to whichever call happens to run first.
func TestTheFirstOfThisSystemsOwnDirectoriesThatHasItWins(t *testing.T) {
	preferred := aDirectoryHolding(t, "ssh-add")
	alsoThere := aDirectoryHolding(t, "ssh-add")
	emulated := aDirectoryHolding(t, "ssh-add", "msys-2.0.dll")
	answers := itself(filepath.Join(preferred, "ssh-add"), filepath.Join(alsoThere, "ssh-add"))
	answers["ssh-add"] = filepath.Join(emulated, "ssh-add")
	answering(t, answers)

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{preferred, alsoThere},
	}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(preferred, "ssh-add"), tool)
}

// TestADirectoryThisSystemNamesButHasNotGotIsPassedOver. The table says where a
// system keeps its own OpenSSH, not that every one of those places exists on
// every machine: a name that resolves to nothing must not end the search.
func TestADirectoryThisSystemNamesButHasNotGotIsPassedOver(t *testing.T) {
	native := aDirectoryHolding(t, "ssh-add")
	emulated := aDirectoryHolding(t, "ssh-add", "msys-2.0.dll")
	answers := itself(filepath.Join(native, "ssh-add"))
	answers["ssh-add"] = filepath.Join(emulated, "ssh-add")
	answering(t, answers)

	tool, err := System{
		EmulationRuntimes: []string{"msys-2.0.dll"},
		NativeDirs:        []string{"", filepath.Join(t.TempDir(), "not-here"), native},
	}.Tool("ssh-add")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(native, "ssh-add"), tool)
}

// TestSSHAddIsTheProgramThisPackageIsAskedAbout keeps the name a constant: it
// is written into the child environments and the messages the user reads, and a
// second spelling of it somewhere else is a program nobody has.
func TestSSHAddIsTheProgramThisPackageIsAskedAbout(t *testing.T) {
	assert.Equal(t, "ssh-add", SSHAddName)
}
