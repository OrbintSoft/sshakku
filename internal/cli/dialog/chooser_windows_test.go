//go:build windows

package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// These verify F29 and F37 on Windows: asked in a box where the session can
// show one, asked on the terminal where it cannot, and never a box where the
// user refused one.
//
// What is installed to draw with is supplied as what it really is — a host on
// PATH, found the way the product finds it. What kind of session this is cannot
// be supplied that way, since it is read from the system rather than from the
// environment, so it is the one thing handed over: a machine somebody is
// sitting at has a screen and a build runner has none, and neither can be asked
// to be the other.

// hostOnPath puts a PowerShell host where the product will look for it. It has
// nothing to run — being installed is what is being arranged here, and nothing
// in these tests draws a box.
func hostOnPath(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	require.NoErrorf(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600), "putting a fake %s on PATH", name)
	t.Setenv("PATH", dir)
}

// inASessionWith arranges the answer the system would give about this session.
func inASessionWith(t *testing.T, screen bool) {
	t.Helper()
	original := thisSessionHasAScreen
	t.Cleanup(func() { thisSessionHasAScreen = original })
	thisSessionHasAScreen = func() bool { return screen }
}

func TestABoxIsRaisedWhereThereIsAScreenAndAHostToDrawIt(t *testing.T) {
	hostOnPath(t, "pwsh.exe")
	inASessionWith(t, true)

	assert.NotNil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session with a screen and a host installed can be shown a box")
}

func TestNoBoxWhereTheSessionHasNoScreen(t *testing.T) {
	hostOnPath(t, "pwsh.exe")
	inASessionWith(t, false)

	assert.Nil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session serving a scheduled job has a window station with no desktop, so there is nowhere to draw")
}

func TestNoBoxWithNothingToDrawItWith(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	inASessionWith(t, true)

	assert.Nil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session that could show a box still needs a host installed to draw one")
}

func TestNoBoxWhenTheUserRefusedOne(t *testing.T) {
	hostOnPath(t, "pwsh.exe")
	inASessionWith(t, true)

	settings := config.Settings{GUIPrompter: config.GUIPrompterNone}
	assert.Nil(t, Graphical(t.Context(), settings, nil),
		"refusing a box is the user's to write, and it holds where one could have been shown")
}

func TestTheBoxNamedIsTheBoxRaised(t *testing.T) {
	hostOnPath(t, "powershell.exe")
	inASessionWith(t, true)

	settings := config.Settings{GUIPrompter: config.GUIPrompterPowerShell}
	got := Graphical(t.Context(), settings, nil)
	require.NotNil(t, got, "naming this platform's box must get that box")
	assert.Equal(t, prompt.PowerShellPrompter{}.Name(), prompt.Name(got),
		"and a message about it names what the user wrote, not something substituted for it")
}

func TestWhatThisSessionReallySaysIsWhatDecides(t *testing.T) {
	// Nothing handed over: the session is read from the system exactly as it is
	// in production. Whichever machine runs this — a desk with a screen, a
	// runner with none — the box must follow that answer rather than the other
	// one, which is what keeps the arrangement above honest.
	hostOnPath(t, "pwsh.exe")

	assert.Equal(t, prompt.GraphicalSession(), Graphical(t.Context(), config.Settings{}, nil) != nil,
		"whether a box comes back is what this session's own window station says")
}
