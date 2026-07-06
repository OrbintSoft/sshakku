package diagnose

import (
	"os"
	"path/filepath"
	"testing"
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
			if ok != c.ok || got != c.want {
				t.Errorf("parseCgroupUnit(%q) = (%q,%v), want (%q,%v)", c.line, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestProcfsCgroupCgroup(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "77")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-gpg-agent.service\n"
	if err := os.WriteFile(filepath.Join(dir, "cgroup"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	unit, ok := ProcfsCgroup{Root: root}.Cgroup(77)
	if !ok || unit != "app-gpg-agent.service" {
		t.Errorf("Cgroup(77) = (%q,%v), want (app-gpg-agent.service,true)", unit, ok)
	}
	if _, ok := (ProcfsCgroup{Root: root}).Cgroup(999); ok {
		t.Error("Cgroup(999) reported ok for a missing process")
	}
}
