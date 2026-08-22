// Package install wires sshakku into a shell's startup files and takes the
// wiring back out again.
//
// The primitives here are a second implementation of shell-hook-lib.sh, which
// is what wires the Unix installs today, and they are held to producing the
// same bytes it does. That is not a stylistic preference: a machine can be
// wired by one and unwired by the other — a Makefile install followed by
// `sshakku uninstall`, or the reverse — and the promise both make is that the
// file comes back exactly as it was found. Two implementations that merely
// agree about what a marker block looks like would leave a stray blank line
// behind, once per install, in somebody's profile.
//
// A block is delimited by two marker lines and holds whatever the caller gives
// it, which is routinely more than one line. Where a drop-in directory exists
// beside the startup file, a small wrapper goes in there instead; the
// directory existing is taken as saying that it is read.
package install

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The lines that delimit sshakku's block. They open with `#`, which is a
// comment in the Bourne shells and in PowerShell alike, so one pair of markers
// serves every startup file this program writes.
const (
	MarkerStart = "# >>> sshakku >>>"
	MarkerEnd   = "# <<< sshakku <<<"
)

// newFileMode is what a startup file this program had to create is given. A
// file it found already there keeps the permissions it had.
const newFileMode fs.FileMode = 0o644

// startupDirMode is what a directory this program had to create to hold a
// startup file is given. The shell opens that file at every login, so the
// account logging in has to be able to enter the directory: one it cannot
// traverse is a wiring that is written and never read.
const startupDirMode fs.FileMode = 0o755

// dropInMode is what a wrapper in a drop-in directory is given, whether it is
// new or is replacing one. Some of the shells that read such a directory run
// each file in it rather than sourcing it, so this one is not negotiable and
// is not inherited from whatever was there before.
const dropInMode fs.FileMode = 0o755

// dropInDirMode is what a drop-in directory this program had to create is
// given. Every session of the scope has to read it at login, and a
// machine-wide one that only its owner could read would take the whole
// directory's worth of wiring with it.
const dropInDirMode fs.FileMode = 0o755

// StripBlock returns content with sshakku's marker block removed, along with
// any blank lines left trailing behind it. Content that never held a block
// comes back with its own trailing blank lines trimmed and a final newline
// assured, which is the same normalisation.
//
// Trimming the trailing blanks is what makes re-running an install
// byte-for-byte idempotent, since UpsertBlock puts a blank line before the
// block it appends: without the trim, each run would leave one more blank line
// than the last, and nothing would ever complain.
func StripBlock(content []byte) []byte {
	kept := make([]string, 0, 16)
	skipping := false
	for _, line := range lines(content) {
		switch line {
		case MarkerStart:
			skipping = true
		case MarkerEnd:
			skipping = false
		default:
			if !skipping {
				kept = append(kept, line)
			}
		}
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == 0 {
		return nil
	}
	var out bytes.Buffer
	for _, line := range kept {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// UpsertBlock returns content with sshakku's block replaced by one wrapping
// body, appended if there was none. A blank line separates the block from
// whatever came before it; content with nothing before it starts straight with
// the marker.
func UpsertBlock(content []byte, body string) []byte {
	var out bytes.Buffer
	if kept := StripBlock(content); len(kept) > 0 {
		out.Write(kept)
		out.WriteByte('\n')
	}
	out.WriteString(MarkerStart)
	out.WriteByte('\n')
	out.WriteString(body)
	out.WriteByte('\n')
	out.WriteString(MarkerEnd)
	out.WriteByte('\n')
	return out.Bytes()
}

// BourneDropIn returns the contents of the small wrapper that goes into a
// drop-in directory: a shell script whose whole job is to reach the installed
// hook, and which says where it came from so that whoever finds it knows it is
// generated rather than hand-written.
func BourneDropIn(body string) []byte {
	return []byte("#!/bin/bash\n" +
		"# sshakku shell hook. Regenerate by re-running the sshakku install.\n" +
		body + "\n")
}

// UpsertBlockFile writes body as sshakku's block in the startup file at path,
// creating the file if it is not there — and the directory that holds it, which
// on an account that has never had a startup file for that shell is not there
// either. A shell names a startup file it will read whether or not anyone has
// made one yet, so the directory being absent is the ordinary state of a
// machine nobody has set up, not a caller's mistake.
func UpsertBlockFile(path, body string) error {
	content, err := read(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), startupDirMode); err != nil {
		return fmt.Errorf("making the directory to hold %s: %w", path, err)
	}
	return replace(path, UpsertBlock(content, body), modeOf(path, newFileMode))
}

// StripBlockFile removes sshakku's block from the startup file at path,
// leaving every other line of it alone. A file that is not there is not an
// error: nothing was wired into it, so there is nothing to unwire, and an
// uninstall must be able to run over a machine that was never installed.
//
// A file with nothing left in it afterwards is removed rather than left empty.
// Such a file held our wiring and nothing else, which means an install created
// it — an account that had no startup file for that shell before has none
// after, which is what putting things back means. A file that had a line of
// somebody's own keeps it, and keeps existing.
func StripBlockFile(path string) error {
	content, err := read(path)
	if err != nil {
		return err
	}
	if content == nil {
		return nil
	}
	stripped := StripBlock(content)
	if len(bytes.TrimSpace(stripped)) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s, which held nothing but the wiring: %w", path, err)
		}
		return nil
	}
	return replace(path, stripped, modeOf(path, newFileMode))
}

// WriteDropIn writes a wrapper into an existing drop-in directory. The
// directory is the caller's to find or create; this only writes the file.
//
// The content is the finished file and is composed by the caller, because the
// caller is the one that knows which language the directory is read in:
// BourneDropIn makes the Bourne one, and a PowerShell drop-in is neither made
// by it nor improved by being given its interpreter line.
func WriteDropIn(path string, content []byte) error {
	return replace(path, content, dropInMode)
}

// RemoveDropIn deletes the wrapper at path. One that is already gone is not an
// error, for the same reason StripBlockFile tolerates a missing file.
//
// A directory there is an error rather than something to delete: this is
// handed a path assembled from a drop-in directory and a file name, so a
// directory at the end of it means the name was wrong, and the one thing worse
// than failing to unwire a hook is removing a directory somebody else put
// there.
func RemoveDropIn(path string) error {
	if info, err := os.Lstat(path); err == nil && info.IsDir() {
		return fmt.Errorf("removing the drop-in hook %s: it is a directory", path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing the drop-in hook %s: %w", path, err)
	}
	return nil
}

// read returns the contents of path, or nil if it is not there.
func read(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return content, nil
}

// modeOf returns the permissions path already has, or fallback where there is
// no file there to have any.
func modeOf(path string, fallback fs.FileMode) fs.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}

// replace puts content at path through a temporary file in the same directory,
// so the file a shell may be reading at that moment is swapped by a rename
// rather than truncated and refilled — there is no instant at which a startup
// file is half of what it was and half of what it is becoming.
//
// The mode is set explicitly and by the caller, because a temporary file is
// created readable only by its owner and it is the file that ends up in place:
// left alone, an atomic write would quietly close a machine-wide startup file
// to every account that has to read it at login.
func replace(path string, content []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("making a temporary file beside %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	// The three below are checked and not one of them is reachable from a test:
	// what would have to fail is writing to, closing, or changing the mode of a
	// file this call created moments earlier in a directory it has already
	// written to — a filled disk, a quota, a mount taken away underneath. They
	// are checked because the rename must not happen after one of them: a
	// startup file replaced by a half-written copy is read at every login, and a
	// machine-wide one replaced by a copy nobody may read is worse.
	if _, err := tmp.Write(content); err != nil {
		//coverage:ignore
		_ = tmp.Close()
		//coverage:ignore
		return fmt.Errorf("writing %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		//coverage:ignore
		return fmt.Errorf("closing %s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		//coverage:ignore
		return fmt.Errorf("setting the permissions of %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// lines splits content the way a line-oriented text tool reads it: on
// newlines, with a single trailing newline ending the last line rather than
// starting an empty one. Nothing at all is no lines, whereas a lone newline is
// one empty line.
func lines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}
