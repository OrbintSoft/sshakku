package keys

import (
	"errors"
	"testing"
)

// namedFake is a prompter that says what it is, the way the real ones do.
type namedFake struct {
	name   string
	answer string
	err    error
	avail  bool
	calls  int
}

func (p *namedFake) Prompt(string) (string, error) { p.calls++; return p.answer, p.err }
func (p *namedFake) Available() bool               { return p.avail }
func (p *namedFake) Name() string                  { return p.name }

func TestFallbackPrompter(t *testing.T) {
	t.Run("an answer from the dialog is the answer", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", answer: "typed in the dialog"}
		terminal := &namedFake{answer: "typed on the terminal"}

		pass, err := FallbackPrompter{Primary: dialog, Fallback: terminal}.Prompt("id_rsa")
		if err != nil || pass != "typed in the dialog" {
			t.Fatalf("Prompt = (%q, %v), want the dialog's answer", pass, err)
		}
		if terminal.calls != 0 {
			t.Errorf("the terminal was asked %d times, want 0: the dialog answered", terminal.calls)
		}
	})

	t.Run("a dialog that will not run asks on the terminal", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", err: errors.New("no such file")}
		terminal := &namedFake{answer: "typed on the terminal"}
		log := &fakeLogger{}

		pass, err := FallbackPrompter{Primary: dialog, Fallback: terminal, Log: log}.Prompt("id_rsa")
		if err != nil || pass != "typed on the terminal" {
			t.Fatalf("Prompt = (%q, %v), want the terminal's answer: being unable to ask must not lose the question", pass, err)
		}
		if !log.contains("pinentry") {
			t.Errorf("log = %v, want the prompter that failed named in it", log.lines)
		}
	})

	t.Run("a dismissed dialog is not asked again elsewhere", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", err: ErrPromptCanceled}
		terminal := &namedFake{answer: "typed on the terminal"}

		_, err := FallbackPrompter{Primary: dialog, Fallback: terminal}.Prompt("id_rsa")
		if !errors.Is(err, ErrPromptCanceled) {
			t.Fatalf("error = %v, want ErrPromptCanceled", err)
		}
		if terminal.calls != 0 {
			t.Errorf("the terminal was asked %d times after the user dismissed the dialog, want 0: cancelling is an answer", terminal.calls)
		}
	})

	t.Run("available while either half can ask", func(t *testing.T) {
		cases := []struct {
			primary, fallback, want bool
		}{
			{true, true, true},
			{false, true, true},
			{true, false, true},
			{false, false, false},
		}
		for _, c := range cases {
			p := FallbackPrompter{
				Primary:  &namedFake{avail: c.primary},
				Fallback: &namedFake{avail: c.fallback},
			}
			if got := p.Available(); got != c.want {
				t.Errorf("Available() with (%v, %v) = %v, want %v", c.primary, c.fallback, got, c.want)
			}
		}
	})
}

func TestPrompterName(t *testing.T) {
	if got := PrompterName(&namedFake{name: "pinentry"}); got != "pinentry" {
		t.Errorf("PrompterName = %q, want %q", got, "pinentry")
	}
	if got := PrompterName(&fakePrompter{}); got == "" {
		t.Error("PrompterName = \"\" for a prompter that does not say what it is, want something a message can use")
	}
}
