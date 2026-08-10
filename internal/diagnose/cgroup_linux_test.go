//go:build linux

package diagnose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCgroupUnit(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{
			"v2 unified, service",
			"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-gpg-agent.service\n",
			"app-gpg-agent.service", true,
		},
		{
			"v2 unified, transient scope",
			"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.gnome.Terminal-12345.scope\n",
			"app-org.gnome.Terminal-12345.scope", true,
		},
		{
			"v1 per-controller, name=systemd line among others",
			"12:pids:/user.slice/user-1000.slice\n" +
				"1:name=systemd:/user.slice/user-1000.slice/user@1000.service/gpg-agent.service\n",
			"gpg-agent.service", true,
		},
		{
			"only slices, no unit",
			"0::/user.slice/user-1000.slice\n",
			"", false,
		},
		{
			"malformed line, no colons",
			"garbage\n",
			"", false,
		},
		{
			"empty",
			"",
			"", false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseCgroupUnit([]byte(c.line))
			assert.Equal(t, c.ok, ok, "whether a unit was found at all")
			assert.Equal(t, c.want, got, "the unit read out of the cgroup line")
		})
	}
}

func TestProcfsCgroupCgroup(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "77")
	require.NoError(t, os.MkdirAll(dir, 0o755), "lay out the fake /proc entry")
	content := "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-gpg-agent.service\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cgroup"), []byte(content), 0o644), "write the fake cgroup file")

	unit, ok := ProcfsCgroup{Root: root}.Cgroup(77)
	assert.True(t, ok, "a process whose cgroup file is there must be answered for")
	assert.Equal(t, "app-gpg-agent.service", unit, "the unit the process runs under")

	_, ok = ProcfsCgroup{Root: root}.Cgroup(999)
	assert.False(t, ok, "a process that is not there must not be answered for")
}

func TestProcfsCgroupReadsTheRealProcfsByDefault(t *testing.T) {
	assert.Equal(t, "/proc", ProcfsCgroup{}.root(),
		"nothing configures a procfs in the doctor, so an unset one has to mean the real one, or the doctor "+
			"reads nobody's cgroup and reports every process as belonging to no unit")
	assert.Equal(t, "/somewhere/else", ProcfsCgroup{Root: "/somewhere/else"}.root(),
		"and a caller that names one is taken at its word")
}
