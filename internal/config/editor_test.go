package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheEditorOpenedIsTheFirstThisSystemHas covers what F36 gives somebody
// who has named no editor anywhere: not one particular program's name, but the
// first of the system's own editors that is actually there.
//
// Each platform's editors and the looking are both handed in, so every
// answer — including the one this machine cannot give — stays checkable from
// here. That is the whole reason the table is data rather than a branch: on a
// Windows Server installation there is no Notepad at all, which is exactly the
// case a machine with Notepad can never exercise.
func TestTheEditorOpenedIsTheFirstThisSystemHas(t *testing.T) {
	windows := []string{"notepad++.exe", "edit.exe", "notepad.exe"}
	unix := []string{"vi"}

	cases := []struct {
		name       string
		candidates []string
		here       map[string]string
		want       string
	}{
		{
			name:       "the one somebody installed comes before the ones that were there anyway",
			candidates: windows,
			here: map[string]string{
				"notepad++.exe": `C:\Program Files\Notepad++\notepad++.exe`,
				"notepad.exe":   `C:\Windows\System32\notepad.exe`,
			},
			want: `C:\Program Files\Notepad++\notepad++.exe`,
		},
		{
			name:       "a machine with only what it ships with",
			candidates: windows,
			here: map[string]string{
				"edit.exe":    `C:\Windows\System32\edit.exe`,
				"notepad.exe": `C:\Windows\System32\notepad.exe`,
			},
			want: `C:\Windows\System32\edit.exe`,
		},
		{
			// A Windows Server installation with no desktop: the editor every
			// other Windows has is the one it has not got.
			name:       "a machine with no Notepad on it at all",
			candidates: windows,
			here:       map[string]string{"edit.exe": `C:\Windows\System32\edit.exe`},
			want:       `C:\Windows\System32\edit.exe`,
		},
		{
			name:       "a machine too old for the console editor",
			candidates: windows,
			here:       map[string]string{"notepad.exe": `C:\Windows\System32\notepad.exe`},
			want:       `C:\Windows\System32\notepad.exe`,
		},
		{
			name:       "the system POSIX requires an editor of",
			candidates: unix,
			here:       map[string]string{"vi": "/usr/bin/vi"},
			want:       "/usr/bin/vi",
		},
		{
			// Nothing to open. Naming what was looked for leaves a program to
			// go and install; saying nothing would leave a command that failed
			// without saying what it wanted.
			name:       "a machine carrying none of them",
			candidates: windows,
			here:       nil,
			want:       "notepad++.exe",
		},
		{
			name:       "nothing to look for either",
			candidates: nil,
			here:       nil,
			want:       "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, firstEditorFound(c.candidates, onAMachineCarrying(c.here)),
				"the editor to open a file in")
		})
	}
}

// TestEditorCommandCutsWhatWasStatedAndNeverWhatWasFound covers the two halves
// of F36's "arguments are passed on": what somebody wrote is a command line and
// is read as one, and what SSHakku went and found is a program's path, which is
// one program however many spaces are in it.
func TestEditorCommandCutsWhatWasStatedAndNeverWhatWasFound(t *testing.T) {
	t.Run("a stated command line is cut into a program and its arguments", func(t *testing.T) {
		assert.Equal(t, []string{"code", "-w"}, EditorCommand(Settings{Editor: "code -w"}),
			"an editor named with arguments must be run with them")
	})

	t.Run("a stated path with spaces in it, in quotes, is one program", func(t *testing.T) {
		assert.Equal(t,
			[]string{`C:\Program Files\Microsoft VS Code\Code.exe`, "-w"},
			EditorCommand(Settings{Editor: `"C:\Program Files\Microsoft VS Code\Code.exe" -w`}),
			"quoting is how a path with a space in it is written in one string")
	})

	t.Run("nothing stated is whatever this machine has, whole", func(t *testing.T) {
		command := EditorCommand(Settings{})
		require.Len(t, command, 1,
			"a path this system was searched for is one program, and is never cut into a command line")
		assert.NotEmpty(t, command[0], "some editor has to be named")
	})
}

// TestThisSystemNamesAnEditorToFallBackOn holds the one thing every platform's
// table must have in common: something to open. An empty table would leave
// `--edit` with nothing to run and nothing to say about it.
func TestThisSystemNamesAnEditorToFallBackOn(t *testing.T) {
	require.NotEmpty(t, platformEditors, "this system must name an editor to fall back on")
}

// TestSplitCommandLine covers the cutting on its own, including what a user can
// write by mistake: an editor named with nothing in it must come back as
// nothing rather than as a program whose name is empty.
func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"a bare program", "vi", []string{"vi"}},
		{"a program and its arguments", "code -w --new-window", []string{"code", "-w", "--new-window"}},
		{"runs of spaces are not arguments", "  code   -w  ", []string{"code", "-w"}},
		{"a tab is a space like any other", "code\t-w", []string{"code", "-w"}},
		{"quotes hold a path together", `"/opt/my editor/ed" -w`, []string{"/opt/my editor/ed", "-w"}},
		{"quotes inside an argument too", `ed --file="a b"`, []string{"ed", "--file=a b"}},
		{"nothing at all", "   ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, splitCommandLine(c.line), "splitCommandLine(%q)", c.line)
		})
	}
}

// onAMachineCarrying stands in for the looking, so a test can describe a
// machine it is not being run on.
func onAMachineCarrying(installed map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		path, ok := installed[name]
		return path, ok
	}
}
