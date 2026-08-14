//go:build linux

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

func TestHasGraphicalSession(t *testing.T) {
	xsetOK := func() *runtest.Runner { return runtest.NewRunner().On("xset", runtest.Stdout("", 0)) }
	xsetDead := func() *runtest.Runner { return runtest.NewRunner().On("xset", runtest.Stdout("", 1)) }
	xsetMissing := func() *runtest.Runner { return runtest.NewRunner().On("xset", runtest.Fails(errors.New("not found"))) }

	cases := []struct {
		name    string
		env     GUIEnv
		runner  *runtest.Runner
		want    bool
		noXcall bool // xset must not be consulted
	}{
		{"wayland short-circuits xset", GUIEnv{WaylandDisplay: "wayland-0"}, xsetDead(), true, true},
		{"x11 live server", GUIEnv{Display: ":0"}, xsetOK(), true, false},
		{"x11 dead server", GUIEnv{Display: ":0"}, xsetDead(), false, false},
		{"x11 no xset binary", GUIEnv{Display: ":0"}, xsetMissing(), false, false},
		{"no display at all", GUIEnv{}, xsetOK(), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, HasGraphicalSession(t.Context(), c.env, c.runner),
				"whether there is a screen decides where the user is asked, and getting it wrong the permissive "+
					"way is a login shell waiting on a dialog nobody will ever see")
			if !c.noXcall {
				return
			}
			for _, call := range c.runner.Calls {
				assert.NotEqual(t, "xset", call.Name,
					"the answer was already settled, and running an X client to confirm it costs the login a process")
			}
		})
	}
}
