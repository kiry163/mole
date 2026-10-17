// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package config

import (
	"os"
	"path/filepath"

	"github.com/kiry163/mole/pkg"
)

// Runtime environment variable keys for the mole process.
// These control the location of files and binaries at runtime and are read
// directly via os.Getenv / os.LookupEnv. All mole-specific keys use the
// MOLE_ prefix. Reference these constants instead of inline string
// literals to keep all supported knobs visible in one place and to prevent
// typos.
const (
	// EnvHome overrides the base directory for all mole data
	// (config, workspace, skills, auth store, …).
	// Default: ~/.mole
	EnvHome = "MOLE_HOME"

	// EnvConfig overrides the full path to the JSON config file.
	// Default: $MOLE_HOME/config.json
	EnvConfig = "MOLE_CONFIG"

	// EnvBuiltinSkills overrides the directory from which built-in
	// skills are loaded.
	// Default: <cwd>/skills
	EnvBuiltinSkills = "MOLE_BUILTIN_SKILLS"
)

// GetHome returns the mole home directory holding config, workspace, skills and
// the auth store.
//
// Priority: $MOLE_HOME > ~/.mole.
func GetHome() string {
	homePath, _ := os.UserHomeDir()
	if explicit := os.Getenv(EnvHome); explicit != "" {
		// An explicit override points at a directory the user manages.
		homePath = explicit
	} else if homePath != "" {
		homePath = filepath.Join(homePath, pkg.DefaultMoleHome)
	}
	if homePath == "" {
		homePath = "."
	}
	return homePath
}
