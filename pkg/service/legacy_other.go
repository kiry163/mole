// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

//go:build !darwin && !linux && !windows

package service

// RemoveLegacyAutostart is a no-op on platforms without a known legacy
// autostart mechanism.
func RemoveLegacyAutostart(string) []error { return nil }
