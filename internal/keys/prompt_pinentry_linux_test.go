//go:build linux

package keys

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakePinentry is the path to a program that speaks the protocol for real, so
// these tests drive a process and a pipe rather than a stand-in for one.
const fakePinentry = "../../test/fakes/pinentry.sh"

func TestPinentryPrompt(t *testing.T) {
	t.Run("returns what was typed", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "correct horse")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt("id_rsa")
		if err != nil {
			t.Fatalf("Prompt = %v, want a passphrase", err)
		}
		if pass != "correct horse" {
			t.Errorf("Prompt = %q, want %q", pass, "correct horse")
		}
	})

	t.Run("a dismissed dialog is ErrPromptCanceled", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_CANCEL", "1")

		if _, err := (PinentryPrompter{Bin: fakePinentry}).Prompt("id_rsa"); !errors.Is(err, ErrPromptCanceled) {
			t.Fatalf("error = %v, want ErrPromptCanceled", err)
		}
	})

	t.Run("status lines and comments are not the answer", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_NOISE", "1")
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "the-passphrase")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt("id_rsa")
		if err != nil {
			t.Fatalf("Prompt = %v, want a passphrase", err)
		}
		if pass != "the-passphrase" {
			t.Errorf("Prompt = %q, want %q: pinentry may say things about itself at any point", pass, "the-passphrase")
		}
	})

	t.Run("percent-escaped bytes come back as they were typed", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "a%25b%0Ac")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt("id_rsa")
		if err != nil {
			t.Fatalf("Prompt = %v, want a passphrase", err)
		}
		if pass != "a%b\nc" {
			t.Errorf("Prompt = %q, want %q", pass, "a%b\nc")
		}
	})

	t.Run("an unanswered dialog does not strand the caller", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_HANG", "1")

		start := time.Now()
		_, err := PinentryPrompter{Bin: fakePinentry, Timeout: 300 * time.Millisecond}.Prompt("id_rsa")
		if err == nil {
			t.Fatal("Prompt = nil error from a dialog that never answered, want the wait to end")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Prompt took %v, want the configured 300ms bound to hold", elapsed)
		}
	})

	t.Run("a pinentry that cannot be started is an error, not a hang", func(t *testing.T) {
		_, err := PinentryPrompter{Bin: "/nonexistent/pinentry"}.Prompt("id_rsa")
		if err == nil {
			t.Fatal("Prompt = nil error with no pinentry to run, want an error")
		}
		if errors.Is(err, ErrPromptCanceled) {
			t.Errorf("error = %v, want a failure to start: nobody dismissed anything", err)
		}
	})
}

// TestPinentryAvailable covers what "there is a pinentry to ask in" has to mean
// for the chain that reads it: not that a program by that name exists, but that
// what it runs can put a dialog on the screen the user is sitting at.
func TestPinentryAvailable(t *testing.T) {
	installed := func(string) (string, error) { return "/usr/bin/pinentry", nil }

	t.Run("not installed", func(t *testing.T) {
		p := PinentryPrompter{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
		if p.Available() {
			t.Error("Available = true with no pinentry on PATH")
		}
	})

	t.Run("a build that draws on a screen", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "gtk2:curses")

		p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
		if !p.Available() {
			t.Error("Available = false for a pinentry that draws with GTK: the console it also falls back to is not what it leads with")
		}
	})

	t.Run("a build that draws on a terminal is not a dialog", func(t *testing.T) {
		for _, flavor := range []string{"curses", "tty"} {
			t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", flavor)

			p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
			if p.Available() {
				t.Errorf("Available = true for the %s build: it would take the prompt from a dialog that can be drawn and then have nowhere to draw it", flavor)
			}
		}
	})

	t.Run("an answer nobody here understands counts as a dialog", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "a-toolkit-nobody-has-written-yet")

		p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
		if !p.Available() {
			t.Error("Available = false for a build this code has never heard of: passing over a dialog that works is the worse mistake")
		}
	})

	t.Run("a pinentry that cannot be asked counts as a dialog", func(t *testing.T) {
		p := PinentryPrompter{Bin: "/nonexistent/pinentry", lookPath: installed}
		if !p.Available() {
			t.Error("Available = false because the question could not be put: too old to answer it is not the same as unable to draw")
		}
	})

	t.Run("a pinentry that never answers does not strand the caller", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_HANG", "1")

		start := time.Now()
		PinentryPrompter{Bin: fakePinentry, lookPath: installed, ProbeTimeout: 300 * time.Millisecond}.Available()
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("Available took %v: asking pinentry about itself waits on no person and must be bounded like any other command", elapsed)
		}
	})
}

// TestAssuanErrorDescribesWhatFailed covers the errors that are not a
// cancellation: what pinentry said has to survive as far as the log, or a
// misconfigured dialog is indistinguishable from one nobody answered.
func TestAssuanErrorDescribesWhatFailed(t *testing.T) {
	t.Run("a cancellation, whichever component reports it", func(t *testing.T) {
		for _, line := range []string{"83886179 Operation cancelled", "83886180 Operation fully cancelled"} {
			if err := assuanError(line); !errors.Is(err, ErrPromptCanceled) {
				t.Errorf("assuanError(%q) = %v, want ErrPromptCanceled", line, err)
			}
		}
	})

	t.Run("anything else keeps its description", func(t *testing.T) {
		err := assuanError("83886254 No pinentry")
		if err == nil || !strings.Contains(err.Error(), "No pinentry") {
			t.Errorf("assuanError = %v, want it to name what failed", err)
		}
	})

	t.Run("a line no number can be read from is still an error", func(t *testing.T) {
		if err := assuanError("something went wrong"); err == nil {
			t.Error("assuanError = nil for an unparseable line, want an error")
		}
	})
}
