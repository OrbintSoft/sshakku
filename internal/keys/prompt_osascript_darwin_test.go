//go:build darwin

package keys

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOsascriptPrompt(t *testing.T) {
	t.Run("returns the entered passphrase, newline trimmed", func(t *testing.T) {
		r := newFakeRunner().on(osascriptBin, stdout("typed-pass\n", 0))
		got, err := OsascriptPrompter{Runner: r}.Prompt("id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "typed-pass" {
			t.Fatalf("passphrase = %q, want typed-pass", got)
		}

		args := r.calls[0].Args
		if len(args) != 2 {
			t.Fatalf("args = %v, want the script and the key name", args)
		}
		// The key name is an argument, not part of the script: a name that
		// closed the string it had been pasted into would otherwise continue
		// as AppleScript of its own.
		if args[1] != "id_rsa" {
			t.Errorf("args[1] = %q, want the key name", args[1])
		}
		if !strings.HasSuffix(args[0], ".applescript") {
			t.Errorf("args[0] = %q, want a script for osascript to run", args[0])
		}
	})

	t.Run("the script really is there while the dialog runs", func(t *testing.T) {
		var body []byte
		r := newFakeRunner().on(osascriptBin, func(c Cmd) (Result, error) {
			body, _ = os.ReadFile(c.Args[0])
			return Result{Stdout: []byte("x")}, nil
		})
		if _, err := (OsascriptPrompter{Runner: r}).Prompt("id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != passphraseDialog {
			t.Errorf("osascript was handed %q, want the embedded dialog", string(body))
		}
	})

	t.Run("the script does not outlive the prompt", func(t *testing.T) {
		var path string
		r := newFakeRunner().on(osascriptBin, func(c Cmd) (Result, error) {
			path = c.Args[0]
			return Result{Stdout: []byte("x")}, nil
		})
		if _, err := (OsascriptPrompter{Runner: r}).Prompt("id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there after the prompt returned", path)
		}
	})

	t.Run("a dismissed dialog is ErrPromptCanceled", func(t *testing.T) {
		r := newFakeRunner().on(osascriptBin, stdout("", 1))
		if _, err := (OsascriptPrompter{Runner: r}).Prompt("id_rsa"); !errors.Is(err, ErrPromptCanceled) {
			t.Fatalf("error = %v, want ErrPromptCanceled", err)
		}
	})

	t.Run("a failure to start osascript is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := newFakeRunner().on(osascriptBin, fails(wantErr))
		if _, err := (OsascriptPrompter{Runner: r}).Prompt("id_rsa"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})

	t.Run("a script that cannot be written is an error, not a silent no-prompt", func(t *testing.T) {
		wantErr := errors.New("no space left")
		restore := writeDialogScript
		defer func() { writeDialogScript = restore }()
		writeDialogScript = func() (string, func(), error) { return "", nil, wantErr }

		r := newFakeRunner().on(osascriptBin, stdout("typed-pass\n", 0))
		if _, err := (OsascriptPrompter{Runner: r}).Prompt("id_rsa"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(r.calls) != 0 {
			t.Error("osascript was run with no script to run")
		}
	})
}

func TestOsascriptAvailable(t *testing.T) {
	found := OsascriptPrompter{lookPath: func(string) (string, error) { return "/usr/bin/osascript", nil }}
	if !found.Available() {
		t.Fatal("Available = false, want true when osascript is on PATH")
	}
	missing := OsascriptPrompter{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
	if missing.Available() {
		t.Fatal("Available = true, want false when osascript is absent")
	}
}

// TestOsascriptAvailableDefaultLookPath covers Available's nil-lookPath branch,
// which falls back to the real os/exec PATH lookup.
func TestOsascriptAvailableDefaultLookPath(t *testing.T) {
	_ = OsascriptPrompter{}.Available()
}

// TestEmbeddedDialogTakesItsArgument pins the contract the Go side depends on:
// the script reads the key name from argv rather than having it built in, and
// hides what is typed. Compiling it is the macOS lint target's job; what is
// checked here is that these two properties are not quietly dropped.
func TestEmbeddedDialogTakesItsArgument(t *testing.T) {
	if !strings.Contains(passphraseDialog, "on run argv") {
		t.Error("the dialog does not take the key name as an argument")
	}
	if !strings.Contains(passphraseDialog, "with hidden answer") {
		t.Error("the dialog would echo the passphrase as it is typed")
	}
}
