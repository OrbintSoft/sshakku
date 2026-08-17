package install

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// fakeAncestry is a fixed pid → parent tree standing in for the system's.
type fakeAncestry map[int]struct {
	ppid int
	name string
}

func (f fakeAncestry) Parent(_ context.Context, pid int) (int, string, bool) {
	entry, ok := f[pid]
	if !ok {
		return 0, "", false
	}
	return entry.ppid, entry.name, true
}

func TestEveryKindThisSystemOffersIsAcceptedByName(t *testing.T) {
	for _, kind := range offeredShellKinds() {
		got, err := ParseShellKind(string(kind))

		require.NoError(t, err)
		assert.Equal(t, kind, got)
	}

	got, err := ParseShellKind("auto")
	require.NoError(t, err)
	assert.Equal(t, Auto, got)
}

// A shell this system cannot wire is refused rather than answered with one it
// can: wiring the wrong shell is worse than wiring none, because the person is
// told it worked.
func TestAShellThisSystemCannotWireIsRefusedAndNamesWhatItCan(t *testing.T) {
	for _, name := range []string{"", "fish", "AUTO", "Bash", "cmd"} {
		_, err := ParseShellKind(name)

		require.Error(t, err, "%q", name)
		assert.Contains(t, err.Error(), string(Auto), "and offers working it out")
		for _, kind := range offeredShellKinds() {
			assert.Contains(t, err.Error(), string(kind))
		}
	}
}

// The kinds on offer are this system's own answer. Windows PowerShell exists
// nowhere but Windows, and offering it elsewhere would accept a flag that can
// only fail later.
func TestTheKindsOfferedAreTheOnesThisSystemHas(t *testing.T) {
	offered := offeredShellKinds()
	require.NotEmpty(t, offered)

	assert.NotContains(t, offered, Auto, "auto is not a shell, it is a way of finding one")
	for _, kind := range offered {
		_, err := ParseShellKind(string(kind))
		assert.NoError(t, err, "a kind this system's table names must be one it accepts")
	}
}

func TestAnInterpreterIsRecognisedByItsImage(t *testing.T) {
	for _, pattern := range shellPatterns() {
		if pattern.confirm != nil {
			continue // needs an environment around it; see below
		}
		kind, ok := RecogniseShell(filepath.Join("somewhere", "else", pattern.base))

		assert.True(t, ok, "%s", pattern.base)
		assert.Equal(t, pattern.kind, kind)
	}
}

func TestSomethingThatIsNotAShellIsNotRecognised(t *testing.T) {
	for _, image := range []string{"", "notepad.exe", "sshakku", filepath.Join("bin", "git")} {
		_, ok := RecogniseShell(image)

		assert.False(t, ok, "%q", image)
	}
}

// Where a name is not enough, something about the program itself is what makes
// it the shell meant, and the evidence is positive rather than a list of
// known-bad places — which would admit every impostor nobody had thought of yet.
//
// What each system looks for is its own to answer and is asked where that answer
// lives. The rule is the same everywhere — a name that needs confirming is not
// taken on the name alone, and one that does not is — so it is checked here,
// against a table written for the purpose.
func TestANameMoreThanOneProgramAnswersToIsConfirmedByItsEnvironment(t *testing.T) {
	asked := ""
	table := []shellPattern{
		{base: "ambiguous", kind: Bash, confirm: func(imagePath string) bool {
			asked = imagePath
			return strings.HasPrefix(filepath.ToSlash(imagePath), "real/")
		}},
		{base: "settled", kind: Zsh},
	}

	kind, ok := recognise(filepath.Join("real", "ambiguous"), table)
	assert.True(t, ok, "a shell with its environment around it is the one meant")
	assert.Equal(t, Bash, kind)
	assert.Equal(t, filepath.Join("real", "ambiguous"), asked,
		"the confirmation is handed the whole path, which is the only thing that tells two of one name apart")

	_, ok = recognise(filepath.Join("impostor", "ambiguous"), table)
	assert.False(t, ok, "the same name with no environment around it is not, and must not be guessed at")

	kind, ok = recognise(filepath.Join("anywhere", "settled"), table)
	assert.True(t, ok, "a name nothing else answers to needs no confirming")
	assert.Equal(t, Zsh, kind)
}

// Typing the command in a window wires that window's shell: the ancestry is
// read upward until something is recognised.
func TestTheShellIsTheNearestOneAboveTheCommand(t *testing.T) {
	shell := shellPatterns()[0]
	tree := fakeAncestry{
		100: {ppid: 90, name: "sshakku.exe"},
		90:  {ppid: 80, name: "some-wrapper"},
		80:  {ppid: 1, name: shell.base},
		1:   {ppid: 0, name: "init"},
	}
	image := func(pid int) (string, bool) {
		if pid == 80 {
			return filepath.Join("anywhere", shell.base), true
		}
		return "", false
	}

	kind, path, err := ResolveShell(t.Context(), 100, tree, image)

	require.NoError(t, err)
	assert.Equal(t, shell.kind, kind)
	assert.Equal(t, filepath.Join("anywhere", shell.base), path,
		"and the interpreter's own path comes back, since that is what gets asked about itself next")
}

// An ancestry that names no shell is reported as that. Picking one anyway is
// the failure this whole path exists to avoid.
func TestAnAncestryWithNoShellInItAsksToBeTold(t *testing.T) {
	tree := fakeAncestry{
		100: {ppid: 90, name: "sshakku.exe"},
		90:  {ppid: 0, name: "some-build-tool"},
	}

	_, _, err := ResolveShell(t.Context(), 100, tree, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--shell", "the command has to say what to do about it")
	assert.Contains(t, err.Error(), "some-build-tool", "and what it looked at")
}

func TestASystemThatSaysNothingAboutAncestryAsksToBeTold(t *testing.T) {
	_, _, err := ResolveShell(t.Context(), 100, fakeAncestry{}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--shell")
	assert.Contains(t, err.Error(), "100")
}

// Where the system cannot give a path, the process table's name is used. That
// settles every kind whose pattern does not ask for confirmation, and honestly
// settles none of the others.
func TestWithNoImagePathTheNameIsUsedForWhatItCanSettle(t *testing.T) {
	for _, pattern := range shellPatterns() {
		tree := fakeAncestry{7: {ppid: 0, name: pattern.base}}

		kind, _, err := ResolveShell(t.Context(), 7, tree, nil)

		if pattern.confirm != nil {
			require.Error(t, err, "%s cannot be settled by name, and must not be", pattern.base)
			continue
		}
		require.NoError(t, err, "%s", pattern.base)
		assert.Equal(t, pattern.kind, kind)
	}
}

var _ launcher.AncestrySource = fakeAncestry{}

// The kinds on offer are the distinct ones this system's table names. A table
// that lists one kind under two names — as a system with two shells of a family
// would — offers it once, because the list is what a refusal reads out and a name
// twice reads as two answers.
func TestAKindNamedTwiceIsOfferedOnce(t *testing.T) {
	assert.True(t, contains([]ShellKind{Bash, Zsh}, Zsh))
	assert.False(t, contains([]ShellKind{Bash, Zsh}, PowerShellCore))
	assert.False(t, contains(nil, Bash))
}
