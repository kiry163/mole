// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// legacyTestPath returns the platform autostart path the test can create.
func legacyTestPath(t *testing.T, home string) string {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/LaunchAgents/io.mole.launcher.plist")
	case "linux":
		return filepath.Join(home, ".config/autostart/mole-web.desktop")
	default:
		t.Skipf("no filesystem legacy autostart on %s", runtime.GOOS)
		return ""
	}
}

func TestRemoveLegacyAutostartRemovesFile(t *testing.T) {
	home := t.TempDir()
	target := legacyTestPath(t, home)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}

	if errs := RemoveLegacyAutostart(home); len(errs) != 0 {
		t.Fatalf("RemoveLegacyAutostart returned errors: %v", errs)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("legacy autostart file still present (stat err: %v)", err)
	}

	// A second run must treat the already-absent entry as success.
	if errs := RemoveLegacyAutostart(home); len(errs) != 0 {
		t.Fatalf("RemoveLegacyAutostart on missing file returned errors: %v", errs)
	}
}

func TestRemoveLegacyAutostartCollectsErrors(t *testing.T) {
	home := t.TempDir()
	target := legacyTestPath(t, home)
	// A non-empty directory cannot be removed by os.Remove, which exercises
	// the error collection path deterministically.
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	errs := RemoveLegacyAutostart(home)
	if len(errs) != 1 {
		t.Fatalf("RemoveLegacyAutostart returned %d errors, want 1: %v", len(errs), errs)
	}
}
