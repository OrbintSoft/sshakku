package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookLib is the shell library this package restates. The tests below are
// written against what it does, and the last of them runs it.
const hookLib = "../../shell-hook-lib.sh"

// sourceLine is the shape a caller actually wires: install-user-hook.sh
// prepends a PATH guard to the source line, so the body of a block is
// routinely more than one line and the primitives must not assume otherwise.
const sourceLine = "case \":$PATH:\" in *\":/home/u/.local/bin:\"*) ;; *) export PATH=\"/home/u/.local/bin:$PATH\" ;; esac\n. \"/home/u/.local/share/sshakku/shell-hook.sh\""

func TestStripBlockRemovesOnlyTheBlockAndItsSeparator(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a file that has no block keeps every line",
			in:   "export EDITOR=vi\nalias ll='ls -l'\n",
			want: "export EDITOR=vi\nalias ll='ls -l'\n",
		},
		{
			name: "the block goes and the lines around it stay",
			in:   "before\n\n# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\nafter\n",
			want: "before\n\nafter\n",
		},
		{
			name: "a block at the end takes the blank line that separated it",
			in:   "before\n\n# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\n",
			want: "before\n",
		},
		{
			name: "a file that is only the block becomes empty",
			in:   "# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\n",
			want: "",
		},
		{
			name: "a last line with no newline gains one",
			in:   "export EDITOR=vi",
			want: "export EDITOR=vi\n",
		},
		{
			name: "an empty file stays empty",
			in:   "",
			want: "",
		},
		{
			name: "trailing blank lines go whether or not there was a block",
			in:   "before\n\n\n\n",
			want: "before\n",
		},
		{
			name: "a marker with anything else on the line is not a marker",
			in:   "# >>> sshakku >>> maybe\nkept\n",
			want: "# >>> sshakku >>> maybe\nkept\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(StripBlock([]byte(tc.in))))
		})
	}
}

func TestUpsertBlockAppendsAfterOneBlankLine(t *testing.T) {
	got := UpsertBlock([]byte("export EDITOR=vi\n"), ". \"/hook.sh\"")
	assert.Equal(t, "export EDITOR=vi\n\n# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\n", string(got))
}

func TestUpsertBlockOnNothingStartsStraightWithTheMarker(t *testing.T) {
	got := UpsertBlock(nil, ". \"/hook.sh\"")
	assert.Equal(t, "# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\n", string(got),
		"a brand-new file has nothing to be separated from")
}

// A second install must leave the file exactly as the first did. Getting this
// wrong is invisible on the run that does it: the file still works, and only
// grows by a line each time it is written.
func TestUpsertBlockRepeatedIsByteForByteTheSame(t *testing.T) {
	once := UpsertBlock([]byte("export EDITOR=vi\n"), sourceLine)
	twice := UpsertBlock(once, sourceLine)
	thrice := UpsertBlock(twice, sourceLine)
	assert.Equal(t, string(once), string(twice))
	assert.Equal(t, string(once), string(thrice))
}

func TestBourneDropInIsAWrapperThatSaysWhereItCameFrom(t *testing.T) {
	got := string(BourneDropIn(". \"/hook.sh\""))
	assert.Equal(t, "#!/bin/bash\n# sshakku shell hook. Regenerate by re-running the sshakku install.\n. \"/hook.sh\"\n", got)
}

func TestUpsertBlockFileCreatesThenReplacesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bash_profile")

	require.NoError(t, UpsertBlockFile(path, ". \"/hook.sh\""))
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# >>> sshakku >>>\n. \"/hook.sh\"\n# <<< sshakku <<<\n", string(first))

	require.NoError(t, UpsertBlockFile(path, ". \"/other.sh\""))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# >>> sshakku >>>\n. \"/other.sh\"\n# <<< sshakku <<<\n", string(second),
		"the block is replaced, not added to")
}

func TestStripBlockFileLeavesTheRestOfTheFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bash_profile")
	original := "export EDITOR=vi\nalias ll='ls -l'\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
	require.NoError(t, UpsertBlockFile(path, ". \"/hook.sh\""))

	require.NoError(t, StripBlockFile(path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "uninstalling gives the file back byte for byte")
}

func TestStripBlockFileOnAFileThatIsNotThereIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-existed")
	require.NoError(t, StripBlockFile(path))
	assert.NoFileExists(t, path, "nothing was wired here, so nothing is created to unwire")
}

func TestDropInIsWrittenAndRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "001-sshakku-init.sh")

	require.NoError(t, WriteDropIn(path, ". \"/hook.sh\""))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(BourneDropIn(". \"/hook.sh\"")), string(got))

	require.NoError(t, RemoveDropIn(path))
	assert.NoFileExists(t, path)
	require.NoError(t, RemoveDropIn(path), "removing what is already gone is what uninstall does twice")
}

// What goes wrong when wiring a startup file is usually the directory: an
// install pointed at a home that is not there, or at a drop-in directory the
// caller believed it had created. The path that failed has to be in the
// message, since the caller passed several.
func TestAFailureNamesTheFileItWasWorkingOn(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "not-a-directory", "profile")

	err := UpsertBlockFile(missing, ". \"/hook.sh\"")
	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)

	err = WriteDropIn(missing, ". \"/hook.sh\"")
	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)

	err = StripBlockFile(dir)
	require.Error(t, err, "a directory is not a startup file, and saying nothing would read as success")
	assert.Contains(t, err.Error(), dir)

	err = UpsertBlockFile(dir, ". \"/hook.sh\"")
	require.Error(t, err)
	assert.Contains(t, err.Error(), dir)

	err = RemoveDropIn(dir)
	require.Error(t, err, "a drop-in directory is not the drop-in file")
	assert.Contains(t, err.Error(), dir)
}

// The claim these primitives make is not that they are reasonable — it is that
// they are the same as shell-hook-lib.sh, which is what wires the Unix
// installs today. Anything else and a file wired by one and unwired by the
// other would not come back as it was. So the library is run, on the same
// inputs, and the bytes are compared.
func TestTheShellLibraryAgreesByteForByte(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash here to compare against")
	}
	lib, err := filepath.Abs(hookLib)
	require.NoError(t, err)
	require.FileExists(t, lib)

	inputs := []struct {
		name string
		file string
	}{
		{name: "a profile with lines of its own", file: "export EDITOR=vi\nalias ll='ls -l'\n"},
		{name: "a profile that already has the block", file: "before\n\n# >>> sshakku >>>\nold\n# <<< sshakku <<<\nafter\n"},
		{name: "a profile ending in the block", file: "before\n\n# >>> sshakku >>>\nold\n# <<< sshakku <<<\n"},
		{name: "a profile with no final newline", file: "export EDITOR=vi"},
		{name: "an empty profile", file: ""},
	}

	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			dir := t.TempDir()

			// upsert-block, compared as the file the library leaves behind.
			path := filepath.Join(dir, "profile")
			require.NoError(t, os.WriteFile(path, []byte(in.file), 0o644))
			run(t, bash, lib, "upsert-block", path, sourceLine)
			theirs, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, string(theirs), string(UpsertBlock([]byte(in.file), sourceLine)))

			// strip-block, which the library prints rather than writes.
			stripPath := filepath.Join(dir, "strip")
			require.NoError(t, os.WriteFile(stripPath, []byte(in.file), 0o644))
			assert.Equal(t, run(t, bash, lib, "strip-block", stripPath), string(StripBlock([]byte(in.file))))
		})
	}

	t.Run("the drop-in wrapper", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "001-sshakku-init.sh")
		run(t, bash, lib, "drop-in", path, sourceLine)
		theirs, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, string(theirs), string(BourneDropIn(sourceLine)))
	})
}

// run invokes the shell library and returns what it printed, failing the test
// with the library's own diagnostics if it did not succeed.
func run(t *testing.T, bash, lib string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bash, append([]string{lib}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, "shell-hook-lib.sh %v: %s", args, stderr.String())
	return string(out)
}
