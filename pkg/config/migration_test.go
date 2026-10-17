package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDropsRemovedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	base := DefaultConfig()
	base.Tools.Exec.TimeoutSeconds = 42
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal default config: %v", err)
	}
	raw["heartbeat"] = map[string]any{"enabled": true, "interval": 30}
	tools := raw["tools"].(map[string]any)
	tools["cron"] = map[string]any{"enabled": true, "exec_timeout_minutes": 10}
	approval := tools["approval"].(map[string]any)
	approval["unattended"] = true
	approval["rules"] = []any{
		map[string]any{"category": "automation", "action": "deny"},
		map[string]any{"tool": "exec", "category": "automation", "action": "deny"},
		map[string]any{"category": "write", "action": "allow"},
	}
	approval["always_allow"] = []any{
		map[string]any{"tool": "cron"},
		map[string]any{"tool": "exec", "arg": "command", "value": "git status"},
	}
	legacy, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal legacy config: %v", err)
	}
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Tools.Approval.Rules) != 2 {
		t.Fatalf("rules = %+v, want the exec tool rule and the write rule", cfg.Tools.Approval.Rules)
	}
	if cfg.Tools.Approval.Rules[0].Tool != "exec" || cfg.Tools.Approval.Rules[1].Category != "write" {
		t.Fatalf("rules = %+v, want exec + write", cfg.Tools.Approval.Rules)
	}
	if len(cfg.Tools.Approval.AlwaysAllow) != 1 || cfg.Tools.Approval.AlwaysAllow[0].Tool != "exec" {
		t.Fatalf("always_allow = %+v, want only the exec entry", cfg.Tools.Approval.AlwaysAllow)
	}
	if cfg.Tools.Exec.TimeoutSeconds != 42 {
		t.Fatalf("tools.exec.timeout_seconds = %d, want 42", cfg.Tools.Exec.TimeoutSeconds)
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	savedData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(savedData, &saved); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if _, ok := saved["heartbeat"]; ok {
		t.Fatal("saved config still has heartbeat")
	}
	savedTools := saved["tools"].(map[string]any)
	if _, ok := savedTools["cron"]; ok {
		t.Fatal("saved config still has tools.cron")
	}
	savedApproval := savedTools["approval"].(map[string]any)
	if _, ok := savedApproval["unattended"]; ok {
		t.Fatal("saved config still has tools.approval.unattended")
	}
	rules := savedApproval["rules"].([]any)
	if len(rules) != 2 ||
		rules[0].(map[string]any)["tool"] != "exec" ||
		rules[1].(map[string]any)["category"] != "write" {
		t.Fatalf("saved rules = %+v, want exec + write", rules)
	}
	allow := savedApproval["always_allow"].([]any)
	if len(allow) != 1 || allow[0].(map[string]any)["tool"] != "exec" {
		t.Fatalf("saved always_allow = %+v, want only the exec entry", allow)
	}
	if got := savedTools["exec"].(map[string]any)["timeout_seconds"]; got != float64(42) {
		t.Fatalf("saved tools.exec.timeout_seconds = %v, want 42", got)
	}
	if _, ok := saved["agents"]; !ok {
		t.Fatal("saved config lost the agents section")
	}
}

func TestLoadConfigMalformedJSONStillReportsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 3,\n  \"heartbeat\": {\"enabled\": true,,\n}\n"
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
