package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

func dialect(t *testing.T, name string) shell.Dialect {
	t.Helper()
	d, err := shell.FromArgs([]string{"--shell=" + name})
	require.NoError(t, err)
	return d
}

func TestThePathOfTheBinaryIsWrittenIntoTheHook(t *testing.T) {
	rendered, err := RenderHook(
		[]byte("before\n$sshakku_bin = '@SSHAKKU_BIN@'\nafter\n"),
		PowerShellBinaryPlaceholder,
		`C:\Users\example\bin\sshakku.exe`,
		dialect(t, shell.PowerShell),
	)

	require.NoError(t, err)
	assert.Equal(t, "before\n$sshakku_bin = 'C:\\Users\\example\\bin\\sshakku.exe'\nafter\n", string(rendered))
	assert.NotContains(t, string(rendered), "@SSHAKKU_BIN@")
}

// An account may be called O'Brien. An apostrophe dropped unescaped into a
// literal ends it, and the rest of the path — and then the rest of the line —
// is read as code by a shell that runs at every login.
func TestAnApostropheInThePathDoesNotEndTheLiteral(t *testing.T) {
	for _, c := range []struct{ name, template, placeholder string }{
		{shell.PowerShell, "$sshakku_bin = '@SSHAKKU_BIN@'\n", PowerShellBinaryPlaceholder},
		{shell.Posix, "sshakku_bin=\"/usr/local/bin/sshakku\"\n", BourneBinaryPlaceholder},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := `C:\Users\O'Brien\bin\sshakku.exe`

			rendered, err := RenderHook([]byte(c.template), c.placeholder, path, dialect(t, c.name))

			require.NoError(t, err)
			line := strings.TrimSuffix(string(rendered), "\n")
			value := line[strings.Index(line, "=")+1:]
			assert.Equal(t, dialect(t, c.name).Quote(path), strings.TrimSpace(value),
				"the value is one literal, whole, in this shell's own quoting")
		})
	}
}

// The curly apostrophes are the surprise: a literal opened with a straight one
// ends at any of them too, and a Windows account directory is exactly where
// they turn up.
func TestTheQuotesThatAreNotApostrophesAreHandledToo(t *testing.T) {
	path := "C:\\Users\\O\u2019Brien\\bin\\sshakku.exe"

	rendered, err := RenderHook(
		[]byte("$sshakku_bin = '@SSHAKKU_BIN@'\n"),
		PowerShellBinaryPlaceholder,
		path,
		dialect(t, shell.PowerShell),
	)

	require.NoError(t, err)
	assert.Contains(t, string(rendered), "\u2019\u2019", "doubled, which is how such a character stands for itself")
}

// A template that has lost its placeholder renders a hook naming some other
// binary, or none. It would be written out, wired up, and quietly do nothing at
// every login.
func TestATemplateWithNowhereToPutThePathIsRefused(t *testing.T) {
	_, err := RenderHook([]byte("nothing to substitute here\n"), PowerShellBinaryPlaceholder, `C:\x`, dialect(t, shell.PowerShell))

	require.Error(t, err)
	assert.Contains(t, err.Error(), PowerShellBinaryPlaceholder)
}

func TestRenderingWithNoBinaryIsRefused(t *testing.T) {
	_, err := RenderHook([]byte("$sshakku_bin = '@SSHAKKU_BIN@'\n"), PowerShellBinaryPlaceholder, "", dialect(t, shell.PowerShell))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binary")
}

// The placeholders are the ones the real templates carry. A constant that
// drifted from the file would refuse every render, which is loud — but only if
// something checks, and this is that something.
func TestThePlaceholdersAreTheOnesTheRealTemplatesCarry(t *testing.T) {
	for _, c := range []struct{ file, placeholder string }{
		{"nn-sshakku-init.ps1", PowerShellBinaryPlaceholder},
		{"nn-ssh-init.sh", BourneBinaryPlaceholder},
	} {
		content, err := os.ReadFile(c.file)
		require.NoError(t, err)

		assert.Contains(t, string(content), c.placeholder, "%s", c.file)
	}
}

// Every occurrence, not the first: a template may name the binary more than
// once, and one left behind would point at a path that does not exist.
func TestEveryOccurrenceIsWrittenOver(t *testing.T) {
	template := "$a = '@SSHAKKU_BIN@'\n$b = '@SSHAKKU_BIN@'\n"

	rendered, err := RenderHook([]byte(template), PowerShellBinaryPlaceholder, `C:\x`, dialect(t, shell.PowerShell))

	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(rendered), `'C:\x'`))
	assert.NotContains(t, string(rendered), "@SSHAKKU_BIN@")
}

// A hook is not written at all when there is nothing to write into it or nowhere
// to put it. Either would leave a file wired into a startup file and running
// something else, or nothing, at every login.
func TestAHookThatCannotBeRenderedOrPlacedIsNotWritten(t *testing.T) {
	t.Run("no binary was named", func(t *testing.T) {
		dir := t.TempDir()

		_, err := renderInto(dir, bournePlan(t), "")

		require.Error(t, err)
		assert.NoFileExists(t, filepath.Join(dir, "shell-hook.sh"))
	})

	t.Run("the directory cannot be made", func(t *testing.T) {
		inTheWay := filepath.Join(t.TempDir(), "a-file-not-a-directory")
		require.NoError(t, os.WriteFile(inTheWay, []byte("mine"), 0o644))

		_, err := renderInto(filepath.Join(inTheWay, "sshakku"), bournePlan(t), "/opt/sshakku/bin/sshakku")

		require.Error(t, err)
		assert.Contains(t, err.Error(), inTheWay)
	})

	t.Run("the hook itself cannot be written", func(t *testing.T) {
		dir := t.TempDir()
		// A directory where the rendered hook belongs, with something in it: it
		// cannot be replaced by a file, and the install has to say so.
		hook := filepath.Join(dir, "shell-hook.sh")
		require.NoError(t, os.Mkdir(hook, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(hook, "something"), []byte("mine"), 0o644))

		_, err := renderInto(dir, bournePlan(t), "/opt/sshakku/bin/sshakku")

		require.Error(t, err)
		assert.Contains(t, err.Error(), hook)
	})
}
