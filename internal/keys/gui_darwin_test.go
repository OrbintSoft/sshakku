//go:build darwin

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGraphicalSession verifies the half of F29 that decides where the asking
// happens. Being on a Mac is not the condition, and getting this wrong in the
// permissive direction is a login shell waiting on a dialog nobody will ever
// see — so every answer that is not plainly "there is a screen" must be read
// as there being none.
func TestGraphicalSession(t *testing.T) {
	cases := []struct {
		name string
		out  string
		code int
		err  error
		want bool
	}{
		{"a login at the machine's own screen", "Aqua\n", 0, nil, true},
		{"no trailing newline", "Aqua", 0, nil, true},
		{"an ssh login", "Background\n", 0, nil, false},
		{"a login shell with no session of its own", "StandardIO\n", 0, nil, false},
		{"a launchd daemon", "System\n", 0, nil, false},
		{"launchctl answered nothing", "", 0, nil, false},
		{"launchctl failed", "Aqua\n", 1, nil, false},
		{"launchctl could not be run at all", "", 0, errors.New("no such file"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newFakeRunner().on(launchctlBin, func(cmd Cmd) (Result, error) {
				assert.Equal(t, []string{"managername"}, cmd.Args,
					"what kind of session this is is the only thing launchctl is being asked")
				return Result{Stdout: []byte(c.out), Code: c.code}, c.err
			})
			assert.Equalf(t, c.want, GraphicalSession(r),
				"being on a Mac is not the condition; every answer that is not plainly \"there is a screen\" must "+
					"be read as there being none, and %q (code %d, err %v) is what was said",
				c.out, c.code, c.err)
		})
	}
}
