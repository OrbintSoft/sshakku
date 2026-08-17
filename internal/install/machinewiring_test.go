package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F19, F44: a machine-wide wiring goes into what every login shell of the system
// reads. Which that is comes from the system's own table; what is done with
// either shape it can name is here, and is checked against a directory and a file
// of the test's own — the real ones belong to the machine running the suite.
func TestAMachineWideWiringGoesWhereEveryLoginShellReads(t *testing.T) {
	t.Run("a directory every login shell reads, one file at a time", func(t *testing.T) {
		p := bournePlan(t)
		dir := filepath.Join(t.TempDir(), "profile.d")

		require.NoError(t, p.forMachine(t.Context(), machineWiring{DropInDir: dir}))

		assert.Equal(t, filepath.Join(dir, p.dropInName()), p.placement.Path)
		assert.True(t, p.placement.DropIn, "a file of ours, not a block in somebody else's")
		assert.Equal(t, []Placement{p.placement}, p.sweep,
			"an uninstall takes away what this install wrote, and nothing in anybody's home")

		// The directory is this install's to make: the shell reads it whether or
		// not anybody has created it, which is the opposite of the rule for the
		// one beside an account's own startup file.
		require.NoError(t, p.writeDropIn(". '/hook.sh'"))
		written, err := os.Stat(p.placement.Path)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o755), written.Mode().Perm())
		made, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o755), made.Mode().Perm(),
			"a directory of wiring nobody but its owner could read would take all of it with it")
	})

	t.Run("a startup file every login shell reads", func(t *testing.T) {
		p := bournePlan(t)
		file := filepath.Join(t.TempDir(), "zprofile")
		require.NoError(t, os.WriteFile(file, []byte("umask 022\n"), 0o644))

		require.NoError(t, p.forMachine(t.Context(), machineWiring{File: file}))

		assert.Equal(t, file, p.placement.Path)
		assert.False(t, p.placement.DropIn, "a marked block inside the system's own file")
		assert.Equal(t, []Placement{p.placement}, p.sweep)

		require.NoError(t, UpsertBlockFile(p.placement.Path, ". '/hook.sh'"))
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Contains(t, string(content), "umask 022", "and what the system had there stays")
	})
}

// A machine-wide target is named in the shell's spelling like every other path
// the shell will read, so a system where the two spellings differ translates it
// first — and cannot go on if that fails.
func TestAMachineWideTargetThatCannotBeTranslatedStopsTheInstall(t *testing.T) {
	spellingThatRefuses(t, false, true)
	p := bournePlan(t)
	p.spelling = spellingForShell("anything")

	assert.Error(t, p.forMachine(t.Context(), machineWiring{DropInDir: "/etc/profile.d"}))
	assert.Error(t, p.forMachine(t.Context(), machineWiring{File: "/etc/zprofile"}))
	assert.Empty(t, p.placement.Path, "and nothing is chosen from a path this program cannot open")
}

// A shell this system has no machine-wide answer for is refused, with the remedy:
// naming the file, or installing for the account instead. Guessing would write a
// block into a file this machine does not read and report it as the wiring.
func TestAShellWithNoMachineWideAnswerIsRefusedWithTheRemedy(t *testing.T) {
	var refused error
	for _, kind := range []ShellKind{Bash, Zsh, PowerShellCore, WindowsPowerShell, Auto} {
		if _, err := machineWiringFor(kind); err != nil {
			refused = err
			assert.Contains(t, err.Error(), string(kind), "the refusal names the shell it is about")
			assert.Contains(t, err.Error(), "--profile", "and what to do instead")
		}
	}
	require.Error(t, refused, "every system this runs on has a shell it cannot answer for machine-wide")
}

// F19, F44: what an install wrote is what an uninstall takes away — a file of
// ours removed, a block inside somebody else's stripped out. Doing the other
// thing to either leaves the hook running under a report that says it is gone:
// stripping a block from a file that is nothing but our own wiring empties
// nothing and removes nothing.
func TestAMachineWideWiringIsTakenBackOutAndNotJustEmptied(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile.d")
	p := bournePlan(t)
	require.NoError(t, p.forMachine(t.Context(), machineWiring{DropInDir: dir}))
	require.NoError(t, p.writeDropIn(". '/hook.sh'"))
	require.FileExists(t, p.placement.Path)

	for _, place := range p.sweep {
		require.NoError(t, p.unwire(place))
	}

	assert.NoFileExists(t, p.placement.Path)
	assert.DirExists(t, dir, "the directory is the system's, and stays")
}

// The two answers nobody should ever see. Both mean this program has contradicted
// itself — a kind that was never settled, or one its own table names and it has no
// wiring for — and the one thing neither may do is write a file anyway.
func TestAKindTheWiringWasNeverDefinedForWritesNothing(t *testing.T) {
	for _, kind := range []ShellKind{Auto, ShellKind("fish")} {
		p := plan{kind: kind}

		err := p.forKind(t.Context(), Request{Scope: User})

		require.Error(t, err)
		assert.Empty(t, p.placement.Path)
		assert.Empty(t, p.sweep)
	}
}
