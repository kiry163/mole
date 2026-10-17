// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

//go:build darwin

package service

import "path/filepath"

// legacyLaunchAgentPath is the LaunchAgent written by the pre-service
// launcher.
const legacyLaunchAgentPath = "Library/LaunchAgents/io.mole.launcher.plist"

// RemoveLegacyAutostart deletes autostart entries left by older mole
// releases. It returns one error per entry that existed but could not be
// removed; missing entries are ignored.
func RemoveLegacyAutostart(home string) []error {
	return removeLegacyFile(filepath.Join(home, legacyLaunchAgentPath))
}
