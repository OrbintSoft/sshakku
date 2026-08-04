//go:build linux

package keys

import (
	"errors"
	"testing"
)

func TestHasGraphicalSession(t *testing.T) {
	xsetOK := func() *fakeRunner { return newFakeRunner().on("xset", stdout("", 0)) }
	xsetDead := func() *fakeRunner { return newFakeRunner().on("xset", stdout("", 1)) }
	xsetMissing := func() *fakeRunner { return newFakeRunner().on("xset", fails(errors.New("not found"))) }

	cases := []struct {
		name    string
		env     GUIEnv
		runner  *fakeRunner
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
			if got := HasGraphicalSession(c.env, c.runner); got != c.want {
				t.Fatalf("HasGraphicalSession = %v, want %v", got, c.want)
			}
			calledXset := false
			for _, call := range c.runner.calls {
				if call.Name == "xset" {
					calledXset = true
				}
			}
			if c.noXcall && calledXset {
				t.Fatal("xset was consulted but should have been short-circuited")
			}
		})
	}
}
