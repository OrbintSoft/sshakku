package install

import (
	"context"
	"os"
	"path/filepath"
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
		if pattern.confirmByTranslator {
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

// Where a name is not enough, being part of a POSIX-emulating environment is
// what makes an interpreter the shell meant — and the evidence is positive: the
// environment's own translator, beside its shell. A list of known-bad places
// would admit every impostor nobody had thought of yet.
func TestANameMoreThanOneProgramAnswersToIsConfirmedByItsEnvironment(t *testing.T) {
	var ambiguous shellPattern
	for _, pattern := range shellPatterns() {
		if pattern.confirmByTranslator {
			ambiguous = pattern
		}
	}
	if ambiguous.base == "" {
		t.Skip("no name on this system is answered to by more than one program")
	}

	// One with an environment around it, one on its own.
	root := t.TempDir()
	real := filepath.Join(root, "bin", ambiguous.base)
	require.NoError(t, os.MkdirAll(filepath.Dir(real), 0o755))
	require.NoError(t, os.WriteFile(real, []byte("a shell"), 0o755))
	translator := cygpathCandidates(real)[len(cygpathCandidates(real))-1]
	require.NoError(t, os.MkdirAll(filepath.Dir(translator), 0o755))
	require.NoError(t, os.WriteFile(translator, []byte("a translator"), 0o755))

	impostor := filepath.Join(t.TempDir(), "system", ambiguous.base)
	require.NoError(t, os.MkdirAll(filepath.Dir(impostor), 0o755))
	require.NoError(t, os.WriteFile(impostor, []byte("a way into somewhere else"), 0o755))

	kind, ok := RecogniseShell(real)
	assert.True(t, ok, "a shell with its environment around it is the one meant")
	assert.Equal(t, ambiguous.kind, kind)

	_, ok = RecogniseShell(impostor)
	assert.False(t, ok, "the same name with no environment around it is not, and must not be guessed at")
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

		if pattern.confirmByTranslator {
			require.Error(t, err, "%s cannot be settled by name, and must not be", pattern.base)
			continue
		}
		require.NoError(t, err, "%s", pattern.base)
		assert.Equal(t, pattern.kind, kind)
	}
}

var _ launcher.AncestrySource = fakeAncestry{}
