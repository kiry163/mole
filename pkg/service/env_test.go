// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envTestPaths(t *testing.T) (configPath, envPath string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "config.json"), filepath.Join(dir, ".env")
}

func TestRefreshEnvUpdatesOnlyListedKeys(t *testing.T) {
	configPath, envPath := envTestPaths(t)
	original := "# keep\nPATH=/old\nSECRET=keep\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/new/bin")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("SECRET", "do-not-import")

	updates, err := RefreshEnv(configPath, true)
	if err != nil {
		t.Fatalf("RefreshEnv(dry-run) error = %v", err)
	}
	if updates["PATH"] != "/new/bin" || updates["LANG"] != "en_US.UTF-8" {
		t.Fatalf("updates = %#v", updates)
	}
	if _, ok := updates["SECRET"]; ok {
		t.Fatal("secret must not be imported")
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("dry-run wrote .env: %q", data)
	}

	if _, err := RefreshEnv(configPath, false); err != nil {
		t.Fatalf("RefreshEnv(write) error = %v", err)
	}
	data, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"# keep", "SECRET=keep", "PATH=/new/bin", "LANG=en_US.UTF-8"} {
		if !strings.Contains(text, want) {
			t.Errorf(".env missing %q:\n%s", want, text)
		}
	}
}

func TestRefreshEnvSkipsAbsentKeys(t *testing.T) {
	configPath, _ := envTestPaths(t)
	old, had := os.LookupEnv("SSL_CERT_DIR")
	if err := os.Unsetenv("SSL_CERT_DIR"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SSL_CERT_DIR", old)
		}
	})

	updates, err := RefreshEnv(configPath, true)
	if err != nil {
		t.Fatalf("RefreshEnv() error = %v", err)
	}
	if _, ok := updates["SSL_CERT_DIR"]; ok {
		t.Fatalf("absent key imported: %#v", updates)
	}
}

func TestEnsureEnvCreatesFromShell(t *testing.T) {
	configPath, envPath := envTestPaths(t)
	t.Setenv("PATH", "/shell/bin")

	if err := EnsureEnv(configPath); err != nil {
		t.Fatalf("EnsureEnv() error = %v", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PATH=/shell/bin") {
		t.Fatalf(".env = %q, want shell PATH", data)
	}
}

func TestEnsureEnvDoesNotOverwriteExisting(t *testing.T) {
	configPath, envPath := envTestPaths(t)
	original := "# custom\nPATH=/keep\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/shell/bin")

	if err := EnsureEnv(configPath); err != nil {
		t.Fatalf("EnsureEnv() error = %v", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("EnsureEnv overwrote existing .env: %q", data)
	}
}

func TestLoadEnvMissingFileIsNoop(t *testing.T) {
	configPath, _ := envTestPaths(t)
	if err := LoadEnv(configPath); err != nil {
		t.Fatalf("LoadEnv() error = %v", err)
	}
}
