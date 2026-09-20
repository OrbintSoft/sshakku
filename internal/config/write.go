package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// keyDirAssignment matches a line that actually sets key_dir, whatever spacing
// it was written with. A commented example is not one: the template ships
// several, and replacing a line that sets nothing would leave the file
// explaining a value it does not set while the real one went somewhere else.
var keyDirAssignment = regexp.MustCompile(`^[ \t]*key_dir[ \t]*=`)

// WithKeyDir returns body with key_dir set to dir — the assignment replaced
// where the file already has one, and a line appended where it has none.
//
// The value is quoted rather than written as it was typed. A path on Windows is
// full of backslashes and a backslash starts an escape inside a TOML string, so
// a directory pasted in unquoted is read back as something else, or as nothing.
func WithKeyDir(body, dir string) string {
	assignment := "key_dir = " + strconv.Quote(dir)

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if keyDirAssignment.MatchString(line) {
			lines[i] = assignment
			return strings.Join(lines, "\n")
		}
	}

	if body == "" {
		return assignment + "\n"
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	// A blank line, so the setting does not read as though the comment block
	// above it were describing it.
	return body + "\n" + assignment + "\n"
}

// SetKeyDir writes key_dir into the user's own config.toml, starting from the
// commented template where they have no file yet.
//
// The file is replaced in one step rather than written in place: a
// configuration truncated halfway is one a login shell reads as a file with a
// syntax error, and the keys stop being loaded for a reason that has nothing to
// do with where they are.
func SetKeyDir(configDir, dir string) error {
	path := MainFile(configDir)

	body := Template()
	switch existing, err := os.ReadFile(path); {
	case err == nil:
		body = string(existing)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", configDir, err)
	}
	staged, err := os.CreateTemp(configDir, MainFileName+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(staged.Name()) }() // Harmless once the rename has taken it.

	if err := writeAndClose(staged, WithKeyDir(body, dir)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(staged.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeAndClose writes the body and closes the file, reporting whichever of the
// two failed. A close that fails after a successful write is still a file that
// may not be on the disk.
func writeAndClose(f *os.File, body string) error {
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// WritableConfig reports whether SetKeyDir could write, without writing: the
// directory is created if it is missing, and a file is made beside the target
// and removed again.
//
// A caller that must not half-do its work needs this answer before it starts,
// and the only way to find out whether a file can be written is to write one.
func WritableConfig(configDir string) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", configDir, err)
	}
	probe, err := os.CreateTemp(configDir, MainFileName+".probe.*")
	if err != nil {
		return fmt.Errorf("write %s: %w", MainFile(configDir), err)
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(probe.Name())
		return fmt.Errorf("write %s: %w", MainFile(configDir), err)
	}
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Dir(probe.Name()), err)
	}
	return nil
}

// KeyDirDecidedElsewhere names the file that decides key_dir instead of the
// user's own config.toml, or "" where nothing does.
//
// Writing config.toml is how a new key directory is made to stick, and a
// config.d drop-in read afterwards would quietly overrule it: the keys would
// move, the file would say so, and every login would go on looking in the old
// place. A caller about to move somebody's keys has to know that first.
func KeyDirDecidedElsewhere(sources []Source, lookup func(string) (string, bool), configDir string) string {
	for _, s := range Explain(sources, lookup) {
		if s.Key != "key_dir" {
			continue
		}
		if s.From.Kind == OriginFile && s.From.Name != MainFile(configDir) {
			return s.From.Name
		}
		return ""
	}
	return ""
}
