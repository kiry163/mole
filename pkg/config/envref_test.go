package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandEnvRefsStringsSlicesMaps(t *testing.T) {
	t.Setenv("TEST_BASE", "https://example.test/v1")
	t.Setenv("TEST_MODEL", "deepseek-chat")
	cfg := DefaultConfig()
	cfg.ModelList = SecureModelList{{
		ModelName:     "m",
		Model:         "$TEST_MODEL",
		APIBase:       "$TEST_BASE",
		CustomHeaders: map[string]string{"X-Ref": "$TEST_BASE"},
	}}
	if err := ExpandEnvRefs(cfg); err != nil {
		t.Fatalf("ExpandEnvRefs() error = %v", err)
	}
	if got := cfg.ModelList[0].Model; got != "deepseek-chat" {
		t.Fatalf("Model = %q", got)
	}
	if got := cfg.ModelList[0].APIBase; got != "https://example.test/v1" {
		t.Fatalf("APIBase = %q", got)
	}
	if got := cfg.ModelList[0].CustomHeaders["X-Ref"]; got != "https://example.test/v1" {
		t.Fatalf("header = %q", got)
	}
}

func TestExpandEnvRefsDoesNotExpandEmbeddedOrRecursively(t *testing.T) {
	t.Setenv("TEST_BASE", "$OTHER")
	cfg := DefaultConfig()
	cfg.ModelList = SecureModelList{{
		ModelName: "m",
		APIBase:   "https://$TEST_BASE/v1",
	}}
	if err := ExpandEnvRefs(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ModelList[0].APIBase; got != "https://$TEST_BASE/v1" {
		t.Fatalf("embedded ref expanded: %q", got)
	}

	cfg.ModelList[0].APIBase = "$TEST_BASE"
	if err := ExpandEnvRefs(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ModelList[0].APIBase; got != "$OTHER" {
		t.Fatalf("recursive expansion: %q", got)
	}
}

func TestExpandEnvRefsMissingFails(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ModelList = SecureModelList{{ModelName: "m", APIBase: "$MISSING_REF"}}
	err := ExpandEnvRefs(cfg)
	if err == nil || !strings.Contains(err.Error(), "MISSING_REF") {
		t.Fatalf("error = %v, want MISSING_REF", err)
	}
}

func TestSecureStringEnvRefAndEscape(t *testing.T) {
	t.Setenv("TEST_SECRET", "s3cret")
	s := NewSecureString("$TEST_SECRET")
	if got := s.String(); got != "s3cret" {
		t.Fatalf("String() = %q", got)
	}
	if s.raw != "$TEST_SECRET" {
		t.Fatalf("raw = %q, want reference", s.raw)
	}
	escaped := NewSecureString("$$TEST_SECRET")
	if got := escaped.String(); got != "$TEST_SECRET" {
		t.Fatalf("escaped String() = %q", got)
	}
	missing := NewSecureString("$NO_SUCH_SECRET")
	if missing.String() != "" {
		t.Fatalf("missing ref resolved to %q", missing.String())
	}
}

func writeRawConfig(t *testing.T, path string, cfg *Config) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestSaveConfigPreservesPlainEnvRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TEST_API_BASE", "https://resolved.test/v1")

	rawCfg := DefaultConfig()
	rawCfg.Channels = ChannelsConfig{}
	rawCfg.ModelList = SecureModelList{{
		ModelName: "ref-model",
		Model:     "openai/gpt-4o",
		APIBase:   "$TEST_API_BASE",
	}}
	writeRawConfig(t, path, rawCfg)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.ModelList[0].APIBase; got != "https://resolved.test/v1" {
		t.Fatalf("resolved APIBase = %q", got)
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(saved), `"api_base": "$TEST_API_BASE"`) {
		t.Fatalf("config lost the reference:\n%s", saved)
	}
	if strings.Contains(string(saved), "https://resolved.test/v1") {
		t.Fatalf("config baked the resolved value:\n%s", saved)
	}
	// The caller keeps resolved runtime values after save.
	if got := cfg.ModelList[0].APIBase; got != "https://resolved.test/v1" {
		t.Fatalf("post-save APIBase = %q, want resolved value", got)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload LoadConfig() error = %v", err)
	}
	if got := reloaded.ModelList[0].APIBase; got != "https://resolved.test/v1" {
		t.Fatalf("reloaded APIBase = %q", got)
	}
}

func TestSaveConfigPreservesPlainEnvRefsWithMultiKeyModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TEST_BASE0", "https://zero.test/v1")
	t.Setenv("TEST_BASE1", "https://one.test/v1")

	rawCfg := DefaultConfig()
	rawCfg.Channels = ChannelsConfig{}
	rawCfg.ModelList = SecureModelList{
		{
			ModelName: "m0",
			Model:     "openai/gpt-4o",
			APIBase:   "$TEST_BASE0",
			APIKeys:   SimpleSecureStrings("key-a", "key-b"),
		},
		{
			ModelName: "m1",
			Model:     "openai/gpt-4o-mini",
			APIBase:   "$TEST_BASE1",
			APIKeys:   SimpleSecureStrings("key-c"),
		},
	}
	writeRawConfig(t, path, rawCfg)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.ModelList) != 3 {
		t.Fatalf("expanded ModelList length = %d, want 3", len(cfg.ModelList))
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	for _, ref := range []string{`"api_base": "$TEST_BASE0"`, `"api_base": "$TEST_BASE1"`} {
		if !strings.Contains(string(saved), ref) {
			t.Fatalf("config lost %s:\n%s", ref, saved)
		}
	}
	for _, resolved := range []string{"https://zero.test/v1", "https://one.test/v1"} {
		if strings.Contains(string(saved), resolved) {
			t.Fatalf("config baked %s:\n%s", resolved, saved)
		}
	}
	// The caller keeps resolved runtime values after save, including virtual
	// multi-key entries.
	if got := cfg.ModelList[0].APIBase; got != "https://zero.test/v1" {
		t.Fatalf("post-save virtual APIBase = %q", got)
	}
}

func TestSaveConfigPreservesEscapedDollarRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	rawCfg := DefaultConfig()
	rawCfg.Channels = ChannelsConfig{}
	rawCfg.ModelList = SecureModelList{{
		ModelName: "escaped-model",
		Model:     "openai/gpt-4o",
		APIBase:   "$$TEST_LITERAL",
	}}
	writeRawConfig(t, path, rawCfg)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.ModelList[0].APIBase; got != "$TEST_LITERAL" {
		t.Fatalf("expanded escape = %q, want $TEST_LITERAL", got)
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), `"api_base": "$$TEST_LITERAL"`) {
		t.Fatalf("escaped reference was not preserved:\n%s", saved)
	}
}

func TestSaveConfigRejectsUndefinedPlainRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TEST_API_BASE", "https://resolved.test/v1")

	rawCfg := DefaultConfig()
	rawCfg.Channels = ChannelsConfig{}
	rawCfg.ModelList = SecureModelList{{
		ModelName: "ref-model",
		Model:     "openai/gpt-4o",
		APIBase:   "$TEST_API_BASE",
	}}
	writeRawConfig(t, path, rawCfg)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	before, _ := os.ReadFile(path)

	cfg.ModelList[0].APIBase = "$MISSING_NEW_REF"
	err = SaveConfig(path, cfg)
	if err == nil || !strings.Contains(err.Error(), "MISSING_NEW_REF") {
		t.Fatalf("SaveConfig() error = %v, want MISSING_NEW_REF", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("SaveConfig wrote the config despite a validation error")
	}
}

func TestSaveConfigKeepsUserEditedResolvedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TEST_API_BASE", "https://resolved.test/v1")

	rawCfg := DefaultConfig()
	rawCfg.Channels = ChannelsConfig{}
	rawCfg.ModelList = SecureModelList{{
		ModelName: "ref-model",
		Model:     "openai/gpt-4o",
		APIBase:   "$TEST_API_BASE",
	}}
	writeRawConfig(t, path, rawCfg)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	cfg.ModelList[0].APIBase = "https://edited.test/v2"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), `"api_base": "https://edited.test/v2"`) {
		t.Fatalf("user edit was overwritten with the reference:\n%s", saved)
	}
}
