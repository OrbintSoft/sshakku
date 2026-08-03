package main

import (
	"fmt"
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// loadSettings reads the TOML config under layout's config dir, resolving it
// against the environment and built-in defaults (environment variable > file >
// default, per setting). config.toml loads first as the base; every *.toml
// file directly under config.d/, in filename order, is then merged on top of
// it, so a drop-in overrides a key config.toml set (see docs/CONFIGURATION.md).
// A missing file or dir is fine; a path, load, or parse problem — including
// one isolated to a single config.d file — is logged under tag and the
// affected setting falls back to its default.
//
// It reads through the same source list `sshakku config` reports from, so what
// that command says is in force is what every other command acts on.
func loadSettings(layout paths.Layout, tag string, log keys.Logger) config.Settings {
	sources := config.LoadSources(layout.ConfigDir)
	for _, source := range sources {
		if source.Err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("%s: config %s: %v", tag, source.Path, source.Err))
		}
	}

	settings, errs := config.Resolve(config.Merged(sources), os.LookupEnv)
	for _, e := range errs {
		_ = log.Log("ERROR", e.Error())
	}
	return settings
}
