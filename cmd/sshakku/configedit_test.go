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
//
// The editor is a real program, not a stand-in: `cp` saves a file over the one
// it was given, which is what an editor does, and it can be told to do so
// without a terminal.
func TestConfigEditOpensTheUsersOwnFile(t *testing.T) {
	tempRuntimeEnv(t)
	saved := writeTemp(t, "saved.toml", "key_lifetime = \"3h\"\n")
	handed := t.TempDir()
	// Two editors in turn: the first keeps a copy of what it was handed, the
	// second saves a file over it. What the user typed and what SSHakku then
	// reads are separate claims, and one run cannot show both.
	t.Setenv("EDITOR", "cp -t "+handed)

	if out, errOut, code := runConfigEdit(t); code != 0 {
		t.Fatalf("exit = %d (%s / %s), want 0", code, out, errOut)
	}

	t.Run("the editor was handed config.toml and nothing else", func(t *testing.T) {
		entries, err := os.ReadDir(handed)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "config.toml" {
			t.Errorf("the editor was handed %v, want config.toml alone", entries)
		}
	})

	t.Run("the file it was handed lists what can be set", func(t *testing.T) {
		created := readFile(t, filepath.Join(handed, "config.toml"))
		for _, key := range []string{"key_lifetime", "secret_backend", "key_patterns"} {
			if !strings.Contains(created, key) {
				t.Errorf("the created file does not mention %s", key)
			}
		}
	})

	t.Run("what the editor saves is what SSHakku reads", func(t *testing.T) {
		t.Setenv("EDITOR", "cp "+saved)
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

// TestConfigEditSaysWhatOverrulesIt verifies the half of F36 that cannot be
// seen from the file being edited: a key config.toml sets that a drop-in
// overrules is an edit with no effect, and the moment to say so is while the
// person who made it is still there.
func TestConfigEditSaysWhatOverrulesIt(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.d/50-work.toml", "key_lifetime = \"2h\"\n")
	saved := writeTemp(t, "saved.toml", "key_lifetime = \"3h\"\n")
	t.Setenv("EDITOR", "cp "+saved)

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
	saved := writeTemp(t, "saved.toml", "key_lifetime = \"eight hours\"\n")
	t.Setenv("EDITOR", "cp "+saved)

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
	saved := writeTemp(t, "broken.toml", "key_lifetime = \n")
	t.Setenv("EDITOR", "cp "+saved)

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
	saved := writeTemp(t, "saved.toml", "max_attempts = 7\n")
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "cp "+saved)

	if _, errOut, code := runConfigEdit(t); code != 0 {
		t.Fatalf("exit = %d (%s), want 0", code, errOut)
	}
	if got := readFile(t, filepath.Join(home, ".config", "sshakku", "config.toml")); !strings.Contains(got, "max_attempts = 7") {
		t.Errorf("config.toml = %q, want what $VISUAL saved", got)
	}
}

// runConfigEdit runs `sshakku config --edit` against the environment the test
// set up, returning stdout, stderr and the exit code.
func runConfigEdit(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := deps{}.run(&stdout, &stderr, []string{"config", "--edit"})
	return stdout.String(), stderr.String(), code
}

// writeTemp writes a file an editor will save over the config with.
func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
