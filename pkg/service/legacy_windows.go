// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

//go:build windows

package service

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	legacyRunKey   = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	legacyRunValue = "MoleLauncher"
)

// RemoveLegacyAutostart deletes the Run entry written by older mole releases.
// A missing value is ignored; other reg.exe failures are returned.
func RemoveLegacyAutostart(string) []error {
	out, err := exec.Command("reg", "delete", legacyRunKey, "/v", legacyRunValue, "/f").CombinedOutput()
	if err == nil {
		return nil
	}
	if isRegValueNotFound(string(out)) {
		return nil
	}
	return []error{fmt.Errorf("remove legacy autostart value %s: %w: %s", legacyRunValue, err, strings.TrimSpace(string(out)))}
}

// isRegValueNotFound reports whether reg.exe failed only because the value is
// absent. reg.exe localizes its message, but the English wording is the common
// case; unknown failures stay visible.
func isRegValueNotFound(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "unable to find") || strings.Contains(lower, "not found")
}
