package keys

import (
	"strings"
	"testing"
)

// explainedFake is a prompter with more than one reason for being unable to
// ask, the way pinentry has.
type explainedFake struct{ namedFake }

func (p *explainedFake) WhyUnavailable() string { return "is not installed, or cannot draw here" }

// TestPrompterUnavailable covers the half of a message the user acts on. Being
// told which dialog could not ask is only useful if what is said about it is
// true of their machine, and most prompters have exactly one thing it can be.
func TestPrompterUnavailable(t *testing.T) {
	t.Run("a program that is either there or not", func(t *testing.T) {
		if got := PrompterUnavailable(&namedFake{name: "kdialog"}); got != "is not installed" {
			t.Errorf("PrompterUnavailable = %q, want the only thing it can be", got)
		}
	})

	t.Run("one that knows better says so itself", func(t *testing.T) {
		got := PrompterUnavailable(&explainedFake{})
		if !strings.Contains(got, "cannot draw here") {
			t.Errorf("PrompterUnavailable = %q, want the prompter's own reason: a sentence that is not true of the machine it is read on is worse than none", got)
		}
	})
}
