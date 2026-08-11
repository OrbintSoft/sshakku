package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err, "a dialog that answered must hand the answer back")
		assert.Equal(t, "typed in the dialog", pass, "and it must be the one typed there")
		assert.Zero(t, terminal.calls, "the question was answered, so the terminal is not asked as well")
	})

	t.Run("a dialog that will not run asks on the terminal", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", err: errors.New("no such file")}
		terminal := &namedFake{name: "the terminal", answer: "typed on the terminal"}
		log := &fakeLogger{}

		pass, err := FallbackPrompter{Primary: dialog, Fallback: terminal, Log: log}.Prompt("id_rsa")
		require.NoError(t, err, "a dialog that could not run must not lose the question")
		assert.Equal(t, "typed on the terminal", pass, "the user is asked on the terminal instead")

		// Both names appear either way round, so what is checked is which of
		// them the line blames: told the terminal failed and pinentry was tried
		// instead, a user goes and looks at the one that works.
		line := strings.Join(log.lines, "\n")
		failed, substitute := strings.Index(line, "pinentry"), strings.Index(line, "the terminal")
		require.GreaterOrEqualf(t, failed, 0, "the log must name the one that could not ask: %v", log.lines)
		require.GreaterOrEqualf(t, substitute, 0, "and where the question went instead: %v", log.lines)
		assert.Lessf(t, failed, substitute,
			"in that order: the one that failed is named first, or the line sends the user to the wrong program: %v",
			log.lines)
	})

	t.Run("a dismissed dialog is not asked again elsewhere", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", err: ErrPromptCanceled}
		terminal := &namedFake{answer: "typed on the terminal"}

		_, err := FallbackPrompter{Primary: dialog, Fallback: terminal}.Prompt("id_rsa")
		assert.ErrorIs(t, err, ErrPromptCanceled, "closing a dialog is an answer, and must be passed on as one")
		assert.Zero(t, terminal.calls,
			"so the same question must not be put again somewhere else the user was not looking")
	})

	t.Run("the log says where the question actually went", func(t *testing.T) {
		dialog := &namedFake{name: "pinentry", err: errors.New("no window server here")}
		// What a session with more than one dialog really hands over to: the
		// rest of the chain, which is asked by asking the dialog at its head.
		other := FallbackPrompter{
			Primary:  &namedFake{name: "zenity", answer: "typed in the other dialog"},
			Fallback: &namedFake{name: "the terminal"},
		}
		log := &fakeLogger{}

		_, err := FallbackPrompter{Primary: dialog, Fallback: other, Log: log}.Prompt("id_rsa")
		require.NoError(t, err, "the rest of the chain must answer")
		// Someone reading the log is trying to find out where they were asked,
		// or why they were not: a line that names a terminal the question never
		// reached sends them looking at the wrong thing.
		assert.Truef(t, log.contains("zenity"),
			"and the log must name where the question actually went: %v", log.lines)
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
			assert.Equalf(t, c.want, p.Available(),
				"a chain can ask while either half can, with the dialog %v and the terminal %v", c.primary, c.fallback)
		}
	})
}

func TestPrompterName(t *testing.T) {
	assert.Equal(t, "pinentry", PrompterName(&namedFake{name: "pinentry"}),
		"a prompter that says what it is called is called that")
	// The terminal is not a program anyone could go and install, so a message
	// that hands the question to it has to name the place rather than a binary
	// the reader would then fail to find.
	assert.Contains(t, PrompterName(TTYPrompter{}), "terminal",
		"the terminal is a place, not a program: naming a binary would send the reader looking for one that does not exist")
	assert.NotEmpty(t, PrompterName(&fakePrompter{}),
		"and one that says nothing about itself still needs something a message can put in front of a user")
}
