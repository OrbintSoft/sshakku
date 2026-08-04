//go:build linux

package keys

import (
	"errors"
	"testing"
)

func TestZenityPrompt(t *testing.T) {
	t.Run("returns the entered passphrase, newline trimmed", func(t *testing.T) {
		r := newFakeRunner().on("zenity", stdout("typed-pass\n", 0))
		got, err := ZenityPrompter{Runner: r}.Prompt("id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "typed-pass" {
			t.Fatalf("passphrase = %q, want typed-pass", got)
		}
		want := []string{"--password", "--title", "Enter passphrase for id_rsa"}
		if a := r.calls[0].Args; !equalStrings(a, want) {
			t.Fatalf("args = %v, want %v", a, want)
		}
	})

	t.Run("a dismissed dialog is ErrPromptCanceled", func(t *testing.T) {
		r := newFakeRunner().on("zenity", stdout("", 1))
		if _, err := (ZenityPrompter{Runner: r}).Prompt("id_rsa"); !errors.Is(err, ErrPromptCanceled) {
			t.Fatalf("error = %v, want ErrPromptCanceled", err)
		}
	})

	t.Run("a failure to start zenity is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := newFakeRunner().on("zenity", fails(wantErr))
		if _, err := (ZenityPrompter{Runner: r}).Prompt("id_rsa"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

func TestZenityAvailable(t *testing.T) {
	found := ZenityPrompter{lookPath: func(string) (string, error) { return "/usr/bin/zenity", nil }}
	if !found.Available() {
		t.Fatal("Available = false, want true when zenity is on PATH")
	}
	missing := ZenityPrompter{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
	if missing.Available() {
		t.Fatal("Available = true, want false when zenity is absent")
	}
}

// TestZenityAvailableDefaultLookPath covers Available's nil-lookPath branch,
// which falls back to the real os/exec PATH lookup. The result depends on
// whether zenity happens to be installed; only the branch matters here.
func TestZenityAvailableDefaultLookPath(t *testing.T) {
	_ = ZenityPrompter{}.Available()
}
