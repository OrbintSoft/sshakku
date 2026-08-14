package config

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/paths"
)

// MainFileName is the single config file, read before any drop-in, and the one
// a user edits as their own.
const MainFileName = "config.toml"

//go:embed template.toml
var templateTOML string

// Template is the configuration file SSHakku writes for a user who has none:
// every setting commented out, with what it does. It is a starting point to
// edit, not a statement of what is in force — nothing reads it back.
func Template() string { return templateTOML }

// MainFile returns the path of the config file under a configuration
// directory, which is where a user's own settings belong.
func MainFile(configDir string) string {
	return filepath.Join(configDir, MainFileName)
}

// dropInDirName holds the drop-ins that apply on top of it, in filename order.
const dropInDirName = "config.d"

// Source is one file the configuration was read from, carrying what was
// decoded from it and what went wrong, if anything. The order sources come in
// is the order they were applied, so the last one to set a key is the one whose
// value is in force.
type Source struct {
	Path string
	File File
	Err  error
}

// LoadSources reads a configuration directory the way a login shell does:
// config.toml first, then every *.toml directly under config.d/ in filename
// order, each overriding the keys it sets on top of what came before.
//
// Only files that are there appear in the result — a file that is absent was
// not read, and a report naming it would say SSHakku consulted something it
// never opened. A file that could not be parsed is listed with its error
// rather than dropped, since a configuration that was ignored is precisely
// what a user needs to be told about.
func LoadSources(configDir string) []Source {
	var sources []Source
	if s, ok := fileSource(MainFile(configDir)); ok {
		sources = append(sources, s)
	}
	return append(sources, dropInSources(filepath.Join(configDir, dropInDirName))...)
}

// Merged applies every source in order, giving the File a caller resolves.
func Merged(sources []Source) File {
	var merged File
	for _, s := range sources {
		merged = merged.Merge(s.File)
	}
	return merged
}

// dropInSources reads the *.toml files directly under dir, in filename order
// (os.ReadDir sorts them). A directory that is not there contributes nothing,
// which is the ordinary case; one that exists and cannot be read — including a
// plain file where somebody meant to make a directory — is reported against
// the directory itself, since no single file can be blamed for it.
func dropInSources(dir string) []Source {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if paths.Absent(dir) {
			return nil
		}
		return []Source{{Path: dir, Err: err}}
	}
	var sources []Source
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		if s, ok := fileSource(filepath.Join(dir, entry.Name())); ok {
			sources = append(sources, s)
		}
	}
	return sources
}

// fileSource decodes one file, reporting whether it is there at all. Load maps
// an absent file to the zero File and no error, which a caller listing what it
// read cannot tell from an empty file that exists, so existence is settled
// first.
func fileSource(path string) (Source, bool) {
	if paths.Absent(path) {
		return Source{}, false
	}
	f, err := Load(path)
	return Source{Path: path, File: f, Err: err}, true
}
