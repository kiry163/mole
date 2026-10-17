// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

//go:build linux

package service

import "path/filepath"

// legacyAutostartPath is the XDG autostart entry written by the pre-service
// launcher.
const legacyAutostartPath = ".config/autostart/mole-web.desktop"

// RemoveLegacyAutostart deletes autostart entries left by older mole
// releases. It returns one error per entry that existed but could not be
// removed; missing entries are ignored.
func RemoveLegacyAutostart(home string) []error {
	return removeLegacyFile(filepath.Join(home, legacyAutostartPath))
}
