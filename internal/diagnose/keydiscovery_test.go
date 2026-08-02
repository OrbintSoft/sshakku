package diagnose

import (
	"errors"
	"strings"
	"testing"
)

// TestReportNamesTheKeyDirectoryItRead covers the part of F34 that makes the
// setting checkable: once the directory can be configured, a report that calls
// it "~/.ssh" is telling the user about a directory it may never have opened.
// A wrong directory and an empty one produce the same list of no keys, and the
// only thing that tells them apart is the name in the heading.
func TestReportNamesTheKeyDirectoryItRead(t *testing.T) {
	const dir = "/home/u/work-keys"

	t.Run("the heading names the directory", func(t *testing.T) {
		ks := &KeySource{Dir: dir, Lister: fakeKeyLister{paths: []string{dir + "/work-github"}}}
		out := formatGathered(t, ks)
		if !strings.Contains(out, dir) {
			t.Errorf("report does not name the directory it read, got:\n%s", out)
		}
		if strings.Contains(out, "~/.ssh") {
			t.Errorf("report names ~/.ssh, which is not where it looked, got:\n%s", out)
		}
	})

	t.Run("a directory that could not be read is named too", func(t *testing.T) {
		ks := &KeySource{Dir: dir, Lister: fakeKeyLister{err: errors.New("no such directory")}}
		out := formatGathered(t, ks)
		if !strings.Contains(out, dir) {
			t.Errorf("failure line does not name the directory, got:\n%s", out)
		}
	})
}

func formatGathered(t *testing.T, ks *KeySource) string {
	t.Helper()
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, fakeSource{}, fakeProber{}, nil, nil, ks, nil)
	var b strings.Builder
	Format(&b, r)
	return b.String()
}
