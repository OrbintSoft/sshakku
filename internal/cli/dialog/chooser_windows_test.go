//go:build windows

package dialog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// These verify F29 and F37 on Windows: asked in a box where the session can
// show one, asked elsewhere where it cannot, and never a box where the user
// refused one.
//
// There is no "nothing installed to draw with" case here, unlike the other two
// platforms. SSHakku draws this box itself, so a session with a screen always
// has something to be asked in — which is the difference this platform's
// prompter was rewritten to make.
//
// What kind of session this is cannot be supplied the way a program on PATH
// can, since it is read from the system rather than from the environment, so it
// is the one thing handed over: a machine somebody is sitting at has a screen
// and a build runner has none, and neither can be asked to be the other.

// inASessionWith arranges the answer the system would give about this session.
func inASessionWith(t *testing.T, screen bool) {
	t.Helper()
	original := thisSessionHasAScreen
	t.Cleanup(func() { thisSessionHasAScreen = original })
	thisSessionHasAScreen = func() bool { return screen }
}

func TestABoxIsRaisedWhereThereIsAScreen(t *testing.T) {
	inASessionWith(t, true)

	assert.NotNil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session with a screen can be shown a box, and needs nothing installed to draw one")
}

func TestNoBoxWhereTheSessionHasNoScreen(t *testing.T) {
	inASessionWith(t, false)

	assert.Nil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session serving a scheduled job has a window station with no desktop, so there is nowhere to draw")
}

func TestNoBoxWhenTheUserRefusedOne(t *testing.T) {
	inASessionWith(t, true)

	settings := config.Settings{GUIPrompter: config.GUIPrompterNone}
	assert.Nil(t, Graphical(t.Context(), settings, nil),
		"refusing a box is the user's to write, and it holds where one could have been shown")
}

func TestTheBoxNamedIsTheBoxRaised(t *testing.T) {
	inASessionWith(t, true)

	settings := config.Settings{GUIPrompter: config.GUIPrompterNative}
	got := Graphical(t.Context(), settings, nil)
	require.NotNil(t, got, "naming this platform's box must get that box")
	assert.Equal(t, prompt.NativePrompter{}.Name(), prompt.Name(got),
		"and a message about it names what the user wrote, not something substituted for it")
}

func TestWhatThisSessionReallySaysIsWhatDecides(t *testing.T) {
	// Nothing handed over: the session is read from the system exactly as it is
	// in production. Whichever machine runs this — a desk with a screen, a
	// runner with none — the box must follow that answer rather than the other
	// one, which is what keeps the arrangement above honest.
	assert.Equal(t, prompt.GraphicalSession(), Graphical(t.Context(), config.Settings{}, nil) != nil,
		"whether a box comes back is what this session's own window station says")
}
