package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

// setupTestEnv points HOME/MOLE_HOME at a temp directory and writes a
// minimal config there. It returns the config path and a cleanup function.
func setupTestEnv(t *testing.T) (string, func()) {
	t.Helper()

	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldMoleHome := os.Getenv("MOLE_HOME")

	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("MOLE_HOME", filepath.Join(tmp, ".mole")); err != nil {
		t.Fatalf("set MOLE_HOME: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "custom-default",
		Model:     "openai/gpt-4o",
		APIKeys:   config.SimpleSecureStrings("sk-default"),
	}}
	cfg.Agents.Defaults.ModelName = "custom-default"

	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	cleanup := func() {
		_ = os.Setenv("HOME", oldHome)
		if oldMoleHome == "" {
			_ = os.Unsetenv("MOLE_HOME")
		} else {
			_ = os.Setenv("MOLE_HOME", oldMoleHome)
		}
	}
	return configPath, cleanup
}
