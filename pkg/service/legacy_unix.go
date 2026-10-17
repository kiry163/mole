// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

//go:build darwin || linux

package service

import (
	"errors"
	"io/fs"
	"os"
)

// removeLegacyFile deletes one autostart entry. A missing file means an older
// release never installed it, which is not an error.
func removeLegacyFile(path string) []error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return []error{err}
	}
	return nil
}
