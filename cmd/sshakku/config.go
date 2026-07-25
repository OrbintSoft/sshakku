package main

import (
	"fmt"
	"os"
	"path/filepath"

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
func loadSettings(layout paths.Layout, tag string, log keys.Logger) config.Settings {
	configPath := filepath.Join(layout.ConfigDir, "config.toml")
	file, err := config.Load(configPath)
	if err != nil {
		_ = log.Log("ERROR", fmt.Sprintf("%s: config %s: %v", tag, configPath, err))
	}

	confDDir := filepath.Join(layout.ConfigDir, "config.d")
	dirFile, dirErrs := config.LoadDir(confDDir)
	for _, e := range dirErrs {
		_ = log.Log("ERROR", fmt.Sprintf("%s: config %s: %v", tag, confDDir, e))
	}
	file = file.Merge(dirFile)

	settings, errs := config.Resolve(file, os.LookupEnv)
	for _, e := range errs {
		_ = log.Log("ERROR", e.Error())
	}
	return settings
}
