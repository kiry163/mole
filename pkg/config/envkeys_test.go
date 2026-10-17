// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetHome covers the two surviving branches of GetHome: an explicit
// MOLE_HOME override is returned verbatim, and an unset/empty one falls back to
// ~/.mole.
func TestGetHome(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home directory: %v", err)
	}

	explicit := t.TempDir()
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"MOLE_HOME used as-is", explicit, explicit},
		{"falls back to ~/.mole", "", filepath.Join(userHome, ".mole")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MOLE_HOME", tc.env)
			if got := GetHome(); got != tc.want {
				t.Fatalf("GetHome() = %q, want %q", got, tc.want)
			}
		})
	}
}
