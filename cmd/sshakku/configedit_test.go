package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigEditOpensTheUsersOwnFile verifies F36 as a user meets it: with no
// configuration at all, `sshakku config --edit` puts a file there listing what
// can be set, hands that file — and no other — to the editor, and what the
// editor saves is what SSHakku reads afterwards.
func TestConfigEditOpensTheUsersOwnFile(t *testing.T) {
	home := tempRuntimeEnv(t)
	record := useEditor(t, "")

	if out, errOut, code := runConfigEdit(t); code != 0 {
		t.Fatalf("exit = %d (%s / %s), want 0", code, out, errOut)
	}

	t.Run("the editor was handed config.toml and nothing else", func(t *testing.T) {
		opened := strings.Fields(readFile(t, record))
		want := filepath.Join(home, ".config", "sshakku", "config.toml")
		if len(opened) != 1 || opened[0] != want {
			t.Errorf("the editor was handed %v, want %s alone", opened, want)
		}
	})

	t.Run("the file it was handed lists what can be set", func(t *testing.T) {
		created := readFile(t, filepath.Join(home, ".config", "sshakku", "config.toml"))
		for _, key := range []string{"key_lifetime", "secret_backend", "key_patterns"} {
			if !strings.Contains(created, key) {
				t.Errorf("the created file does not mention %s", key)
			}
		}
	})

	t.Run("what the editor saves is what SSHakku reads", func(t *testing.T) {
		useEditor(t, "key_lifetime = \"3h\"\n")
		if _, errOut, code := runConfigEdit(t); code != 0 {
			t.Fatalf("exit = %d (%s), want 0", code, errOut)
		}
		out, _, code := runConfig(t)
		if code != 0 {
			t.Fatalf("config exit = %d, want 0", code)
		}
		if line := settingLine(t, out, "key_lifetime"); !strings.Contains(line, "3h") {
			t.Errorf("%q is not what the editor saved", line)
		}
	})
}

// TestConfigEditPassesOnTheEditorsOwnArguments covers the shape $EDITOR really
// has: people set it to a command line ("code -w", "emacs -nw"), and running
// only its first word starts some editors in a mode their owner never uses.
func TestConfigEditPassesOnTheEditorsOwnArguments(t *testing.T) {
	tempRuntimeEnv(t)
	record := useEditor(t, "")
	t.Setenv("EDITOR", editorScript(t)+" --wait --new-window")

	if _, errOut, code := runConfigEdit(t); code != 0 {
		t.Fatalf("exit = %d (%s), want 0", code, errOut)
	}
	opened := strings.Fields(readFile(t, record))
	if len(opened) != 3 || opened[0] != "--wait" || opened[1] != "--new-window" {
		t.Errorf("the editor was run as %v, want its own arguments before the path", opened)
	}
}

// TestConfigEditSaysWhatOverrulesIt verifies the half of F36 that cannot be
// seen from the file being edited: a key config.toml sets that a drop-in
// overrules is an edit with no effect, and the moment to say so is while the
// person who made it is still there.
func TestConfigEditSaysWhatOverrulesIt(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.d/50-work.toml", "key_lifetime = \"2h\"\n")
	useEditor(t, "key_lifetime = \"3h\"\n")

	out, errOut, code := runConfigEdit(t)
	if code != 0 {
		t.Fatalf("exit = %d (%s), want 0: an overruled key is not a failure", code, errOut)
	}
	said := out + errOut
	if !strings.Contains(said, "key_lifetime") {
		t.Errorf("nothing names the key that was overruled:\n%s", said)
	}
	if !strings.Contains(said, "50-work.toml") {
		t.Errorf("nothing names the file that overrules it:\n%s", said)
	}
}

// TestConfigEditReportsAValueThatWillBeIgnored verifies F36 for the file that
// parses perfectly and still does nothing: a value SSHakku will not use is
// exactly as silent as one a drop-in overrules, and it is silent in the file
// the user is looking at.
func TestConfigEditReportsAValueThatWillBeIgnored(t *testing.T) {
	tempRuntimeEnv(t)
	useEditor(t, "key_lifetime = \"eight hours\"\n")

	out, errOut, code := runConfigEdit(t)
	if code != 0 {
		t.Fatalf("exit = %d, want 0: the file is readable, one value in it is not usable", code)
	}
	said := out + errOut
	if !strings.Contains(said, "key_lifetime") || !strings.Contains(said, "eight hours") {
		t.Errorf("nothing says the value will be ignored:\n%s", said)
	}
}

// TestConfigEditReportsAFileThatNoLongerParses verifies the other half of F36:
// a syntax error discards the whole file at the next login, silently. Told at
// once, with a non-zero exit, it is a mistake the user can still fix.
func TestConfigEditReportsAFileThatNoLongerParses(t *testing.T) {
	tempRuntimeEnv(t)
	useEditor(t, "key_lifetime = \n")

	out, errOut, code := runConfigEdit(t)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for a file that will be discarded", code)
	}
	if !strings.Contains(out+errOut, "config.toml") {
		t.Errorf("nothing names the file that can no longer be read:\n%s%s", out, errOut)
	}
}

// TestConfigEditWithNoEditorToRun covers the editor that is not there: naming
// it beats a silent exit, since $EDITOR is the one thing the user can correct.
func TestConfigEditWithNoEditorToRun(t *testing.T) {
	tempRuntimeEnv(t)
	t.Setenv("EDITOR", "sshakku-no-such-editor")

	_, errOut, code := runConfigEdit(t)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 when the editor could not be run", code)
	}
	if !strings.Contains(errOut, "sshakku-no-such-editor") {
		t.Errorf("stderr %q does not name the editor it tried to run", errOut)
	}
}

// TestConfigEditFallsBackToVisual covers the second variable: a user who set
// only $VISUAL still gets an editor rather than whatever vi does on a machine
// that has none.
func TestConfigEditFallsBackToVisual(t *testing.T) {
	home := tempRuntimeEnv(t)
	useEditor(t, "max_attempts = 7\n")
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", editorScript(t))

	if _, errOut, code := runConfigEdit(t); code != 0 {
		t.Fatalf("exit = %d (%s), want 0", code, errOut)
	}
	if got := readFile(t, filepath.Join(home, ".config", "sshakku", "config.toml")); !strings.Contains(got, "max_attempts = 7") {
		t.Errorf("config.toml = %q, want what $VISUAL saved", got)
	}
}

// useEditor points $EDITOR at the stand-in under testdata: a real program,
// exec'd by SSHakku like any editor, which records what it was asked to open
// and saves body over it (empty body leaves the file untouched). It returns the
// path of the record.
//
// It is a script rather than a stock utility because those differ between the
// systems this suite runs on — BSD `cp` has no `-t` — and an editor is the one
// thing here that has to behave the same on both.
func useEditor(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "opened")
	t.Setenv("EDITOR", editorScript(t))
	t.Setenv("SSHAKKU_TEST_EDITOR_RECORD", record)
	t.Setenv("SSHAKKU_TEST_EDITOR_BODY", "")
	if body != "" {
		saved := filepath.Join(dir, "saved.toml")
		if err := os.WriteFile(saved, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SSHAKKU_TEST_EDITOR_BODY", saved)
	}
	return record
}

// editorScript is the absolute path of the stand-in editor, which SSHakku runs
// from the user's own working directory rather than this package's.
func editorScript(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "editor.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// runConfigEdit runs `sshakku config --edit` against the environment the test
// set up, returning stdout, stderr and the exit code.
func runConfigEdit(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := deps{}.run(&stdout, &stderr, []string{"config", "--edit"})
	return stdout.String(), stderr.String(), code
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
