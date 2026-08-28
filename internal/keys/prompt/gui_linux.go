//go:build linux

package prompt

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// GUIEnv is the subset of the environment that GUI detection reads. Both
// variables are freedesktop display-server conventions, which is why this
// lives on the Linux side: no macOS session advertises itself this way.
type GUIEnv struct {
	WaylandDisplay string // $WAYLAND_DISPLAY.
	Display        string // $DISPLAY.
}

// HasGraphicalSession reports whether a display server is reachable: a Wayland
// compositor advertised by WAYLAND_DISPLAY, or an X server that answers `xset q`.
// Checking xset (rather than DISPLAY alone) rejects a stale DISPLAY pointing at a
// dead server; a missing xset binary is treated as no X session.
//
// It answers only whether there is a screen. Whether anything is installed to
// draw a dialog on it is a separate question, asked of the dialogs themselves.
func HasGraphicalSession(ctx context.Context, env GUIEnv, r run.Runner) bool {
	if env.WaylandDisplay != "" {
		return true
	}
	if env.Display == "" {
		return false
	}
	res, err := r.Run(ctx, run.Cmd{Name: "xset", Args: []string{"q"}})
	return err == nil && res.Code == 0
}
