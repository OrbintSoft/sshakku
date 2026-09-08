//go:build windows

package config

import (
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// platformEditors are the editors to open a file in where nobody named one,
// best first: the first of them this system has is the one used.
//
// Notepad++ is not this system's own, and it comes first because of that: a
// program somebody installed to edit text with is a better guess at what they
// want than one that was there whether they wanted it or not. Microsoft Edit
// is next — recent builds carry it, so do the Server editions that have no
// desktop at all, and it edits in the console the command was typed in.
// Notepad is last: every Windows with a desktop has it, and only those.
var platformEditors = []string{"notepad++.exe", "edit.exe", "notepad.exe"}

// findEditor says where a program of this name can be run from, or that it
// cannot be.
//
// PATH is not the whole answer on this system. A program that installs itself
// outside PATH records where it went under App Paths, which is the key the
// system's own Run box resolves a bare name through — Notepad++ is one such,
// so PATH alone would miss it on every machine that has it.
func findEditor(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	return appPath(name)
}

// appPath reads where this system says a program of that name was installed,
// and reports it only if something is still there: an entry outlives the
// program it names, and a path to nothing would be run and fail rather than
// passed over for the next editor.
func appPath(name string) (string, bool) {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root,
			`SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := key.GetStringValue("")
		_ = key.Close()
		if err != nil {
			continue
		}
		// Some installers quote the value and some do not.
		path = strings.Trim(strings.TrimSpace(path), `"`)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		return path, true
	}
	return "", false
}
