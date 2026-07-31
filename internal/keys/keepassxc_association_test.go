package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

func TestFileAssociationRoundTrip(t *testing.T) {
	store := FileAssociationStore{Path: filepath.Join(t.TempDir(), "nested", "assoc.json")}
	want := keepassxc.Association{ID: "db-1", IDKey: "a-public-key"}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !found {
		t.Fatal("what was saved must be found")
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

// TestFileAssociationIsNotWorldReadable checks the permissions rather than
// trusting the constant: anyone who could read this file could present
// themselves to KeePassXC as SSHakku.
func TestFileAssociationIsNotWorldReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assoc.json")
	store := FileAssociationStore{Path: path}
	if err := store.Save(keepassxc.Association{ID: "db", IDKey: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("mode = %o, want nothing for group or other", perm)
	}
}

// TestFileAssociationMissingIsNotAnError covers the state every user starts in.
func TestFileAssociationMissingIsNotAnError(t *testing.T) {
	store := FileAssociationStore{Path: filepath.Join(t.TempDir(), "absent.json")}
	_, found, err := store.Load()
	if err != nil {
		t.Fatalf("a missing association must not be an error: %v", err)
	}
	if found {
		t.Error("nothing was saved, so nothing must be found")
	}
}

func TestFileAssociationRejectsUnreadableContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not JSON at all", "{{{"},
		{"a version this build does not know", `{"version":99,"id":"db","idKey":"k"}`},
		{"no database id", `{"version":1,"idKey":"k"}`},
		{"no identification key", `{"version":1,"id":"db"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "assoc.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("writing: %v", err)
			}
			_, found, err := FileAssociationStore{Path: path}.Load()
			if err == nil {
				t.Fatal("an association that cannot be understood must be reported, not silently ignored")
			}
			if found {
				t.Error("nothing usable was read, so found must be false")
			}
		})
	}
}

func TestFileAssociationReportsAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// A directory where the file should be: readable as a path, not as a file.
	path := filepath.Join(dir, "assoc.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, _, err := (FileAssociationStore{Path: path}).Load(); err == nil {
		t.Fatal("a file that cannot be read must be reported")
	}
}

func TestFileAssociationReportsAnUnwritableLocation(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	// The parent of the target is a regular file, so the directory cannot be
	// created.
	store := FileAssociationStore{Path: filepath.Join(blocker, "assoc.json")}
	if err := store.Save(keepassxc.Association{ID: "db", IDKey: "k"}); err == nil {
		t.Fatal("an association that could not be written must be reported")
	}
}

func TestFileAssociationReportsAnUnwritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assoc.json")
	// A directory at the target path: the directory creation succeeds, the
	// file write does not.
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := (FileAssociationStore{Path: path}).Save(keepassxc.Association{ID: "db", IDKey: "k"}); err == nil {
		t.Fatal("a write that could not land must be reported")
	}
}
