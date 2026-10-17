package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMalformedJSONStillReportsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 4,\n  \"heartbeat\": {\"enabled\": true,,\n}\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected syntax error, got nil")
	}
	if !strings.Contains(err.Error(), "config.json syntax error at line 3") {
		t.Fatalf("expected config.json line diagnostic, got %q", err.Error())
	}
}
