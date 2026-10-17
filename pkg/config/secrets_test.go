package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigPromotesPlaintextSecretToEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Dashboard.Token = *NewSecureString("dash-secret")
	cfg.Channels = ChannelsConfig{}
	cfg.ModelList = SecureModelList{}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	if !strings.Contains(string(rawConfig), `"$DASHBOARD_TOKEN"`) {
		t.Fatalf("config missing reference:\n%s", rawConfig)
	}
	if strings.Contains(string(rawConfig), "dash-secret") {
		t.Fatalf("config leaked secret:\n%s", rawConfig)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	if !strings.Contains(string(rawEnv), "DASHBOARD_TOKEN=dash-secret") {
		t.Fatalf(".env missing secret:\n%s", rawEnv)
	}
	if got := os.Getenv("DASHBOARD_TOKEN"); got != "dash-secret" {
		t.Fatalf("DASHBOARD_TOKEN = %q, want dash-secret", got)
	}
}

func TestSaveConfigPreservesUserEnvLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	envPath := EnvFilePath(path)
	if err := os.WriteFile(envPath, []byte("PATH=/bin\n# user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Dashboard.Token = *NewSecureString("dash-secret")
	cfg.ModelList = SecureModelList{}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	rawEnv, _ := os.ReadFile(envPath)
	got := string(rawEnv)
	for _, keep := range []string{"PATH=/bin\n", "# user\n"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost user env line %q:\n%s", keep, got)
		}
	}
}

func TestSecurityCopyFromCopiesReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	existing := DefaultConfig()
	existing.Dashboard.Token = *NewSecureString("dash-secret")
	existing.ModelList = SecureModelList{}
	if err := SaveConfig(path, existing); err != nil {
		t.Fatal(err)
	}
	fresh := DefaultConfig()
	fresh.ModelList = SecureModelList{}
	if err := fresh.SecurityCopyFrom(path); err != nil {
		t.Fatal(err)
	}
	if fresh.Dashboard.Token.String() != "dash-secret" {
		t.Fatalf("copied token = %q", fresh.Dashboard.Token.String())
	}
	if fresh.Dashboard.Token.raw != "$DASHBOARD_TOKEN" {
		t.Fatalf("copied raw = %q", fresh.Dashboard.Token.raw)
	}
}

func TestResetToDefaultsPreservesSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Dashboard.Token = *NewSecureString("dash-secret")
	cfg.ModelList = SecureModelList{}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if err := ResetToDefaults(path); err != nil {
		t.Fatalf("ResetToDefaults() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	if !strings.Contains(string(rawConfig), `"$DASHBOARD_TOKEN"`) {
		t.Fatalf("reset config missing reference:\n%s", rawConfig)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Dashboard.Token.String(); got != "dash-secret" {
		t.Fatalf("token after reset = %q, want dash-secret", got)
	}
}

func TestSaveConfigKeepsEnvValueEditedByUser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("DASHBOARD_TOKEN", "from-process")
	if err := os.WriteFile(path, []byte(`{"version": 4, "dashboard": {"token": "$DASHBOARD_TOKEN"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EnvFilePath(path), []byte("DASHBOARD_TOKEN=from-env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ModelList = SecureModelList{}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	if !strings.Contains(string(rawEnv), "DASHBOARD_TOKEN=from-env") {
		t.Fatalf(".env value overwritten:\n%s", rawEnv)
	}
	if strings.Contains(string(rawEnv), "from-process") {
		t.Fatalf(".env should keep the file value:\n%s", rawEnv)
	}
}

func TestSaveConfigPromotesDollarPrefixedPlaintext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Dashboard.Token = *NewSecureString("$ecret-literal")
	cfg.Channels = ChannelsConfig{}
	cfg.ModelList = SecureModelList{}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	if !strings.Contains(string(rawConfig), `"$DASHBOARD_TOKEN"`) {
		t.Fatalf("config missing reference:\n%s", rawConfig)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	if !strings.Contains(string(rawEnv), "DASHBOARD_TOKEN=$ecret-literal") {
		t.Fatalf(".env missing literal:\n%s", rawEnv)
	}
}

func TestSaveConfigPromotesChannelToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.ModelList = SecureModelList{}
	telegram := cfg.Channels.Get("telegram")
	if telegram == nil {
		t.Fatal("DefaultConfig() missing telegram channel")
	}
	if err := telegram.Decode(&TelegramSettings{Token: *NewSecureString("bot-secret")}); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	if !strings.Contains(string(rawConfig), `"$TELEGRAM_TOKEN"`) {
		t.Fatalf("config missing channel reference:\n%s", rawConfig)
	}
	if strings.Contains(string(rawConfig), "bot-secret") {
		t.Fatalf("config leaked channel secret:\n%s", rawConfig)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	if !strings.Contains(string(rawEnv), "TELEGRAM_TOKEN=bot-secret") {
		t.Fatalf(".env missing channel secret:\n%s", rawEnv)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := reloaded.Channels.Get("telegram").GetDecoded()
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.(*TelegramSettings).Token.String(); got != "bot-secret" {
		t.Fatalf("reloaded token = %q", got)
	}
}

func TestSecurityCopyFromCopiesChannelToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	existing := DefaultConfig()
	existing.ModelList = SecureModelList{}
	telegram := existing.Channels.Get("telegram")
	if telegram == nil {
		t.Fatal("DefaultConfig() missing telegram channel")
	}
	if err := telegram.Decode(&TelegramSettings{Token: *NewSecureString("bot-secret")}); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, existing); err != nil {
		t.Fatal(err)
	}

	// Simulate a console payload: the channel is present but its secret is not.
	fresh := DefaultConfig()
	fresh.ModelList = SecureModelList{}
	fresh.Channels = ChannelsConfig{
		"telegram": {
			Enabled:  true,
			Type:     ChannelTelegram,
			Settings: mustParseRawNode(`{"base_url":"https://api.telegram.org"}`),
		},
	}
	if err := fresh.SecurityCopyFrom(path); err != nil {
		t.Fatal(err)
	}
	decoded, err := fresh.Channels.Get("telegram").GetDecoded()
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.(*TelegramSettings).Token.String(); got != "bot-secret" {
		t.Fatalf("copied channel token = %q, want bot-secret", got)
	}
}

func TestSaveConfigPromotesDuplicateModelNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Channels = ChannelsConfig{}
	cfg.ModelList = SecureModelList{
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-a")},
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-b")},
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	for _, ref := range []string{`"$GPT_4O_API_KEY"`, `"$GPT_4O__DUP2_API_KEY"`} {
		if !strings.Contains(string(rawConfig), ref) {
			t.Fatalf("config missing %s:\n%s", ref, rawConfig)
		}
	}
	if strings.Contains(string(rawConfig), "key-a") || strings.Contains(string(rawConfig), "key-b") {
		t.Fatalf("config leaked model keys:\n%s", rawConfig)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	for _, want := range []string{"GPT_4O_API_KEY=key-a", "GPT_4O__DUP2_API_KEY=key-b"} {
		if !strings.Contains(string(rawEnv), want) {
			t.Fatalf(".env missing %q:\n%s", want, rawEnv)
		}
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ModelList) != 2 {
		t.Fatalf("reloaded %d models, want 2", len(reloaded.ModelList))
	}
	if got := reloaded.ModelList[0].APIKey(); got != "key-a" {
		t.Fatalf("first model key = %q, want key-a", got)
	}
	if got := reloaded.ModelList[1].APIKey(); got != "key-b" {
		t.Fatalf("second model key = %q, want key-b", got)
	}
}

func TestSaveConfigPromotesDuplicateNamesWithoutCollidingWithNamedDuplicate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Channels = ChannelsConfig{}
	cfg.ModelList = SecureModelList{
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-a")},
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-b")},
		{ModelName: "gpt-4o-2", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-c")},
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	for _, want := range []string{
		"GPT_4O_API_KEY=key-a",
		"GPT_4O__DUP2_API_KEY=key-b",
		"GPT_4O_2_API_KEY=key-c",
	} {
		if !strings.Contains(string(rawEnv), want) {
			t.Fatalf(".env missing %q:\n%s", want, rawEnv)
		}
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ModelList) != 3 {
		t.Fatalf("reloaded %d models, want 3", len(reloaded.ModelList))
	}
	for i, want := range []string{"key-a", "key-b", "key-c"} {
		if got := reloaded.ModelList[i].APIKey(); got != want {
			t.Fatalf("model %d key = %q, want %q", i, got, want)
		}
	}
}

func TestSaveConfigPromotesModelNameStartingWithDigit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := DefaultConfig()
	cfg.Channels = ChannelsConfig{}
	cfg.ModelList = SecureModelList{
		{ModelName: "4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("digit-key")},
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	rawConfig, _ := os.ReadFile(path)
	if !strings.Contains(string(rawConfig), `"$SECRET_4O_API_KEY"`) {
		t.Fatalf("config missing reference:\n%s", rawConfig)
	}
	rawEnv, _ := os.ReadFile(EnvFilePath(path))
	if !strings.Contains(string(rawEnv), "SECRET_4O_API_KEY=digit-key") {
		t.Fatalf(".env missing secret:\n%s", rawEnv)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ModelList[0].APIKey(); got != "digit-key" {
		t.Fatalf("reloaded key = %q, want digit-key", got)
	}
}

func TestSecurityCopyFromPreservesDuplicateModelKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	existing := DefaultConfig()
	existing.Channels = ChannelsConfig{}
	existing.ModelList = SecureModelList{
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-a")},
		{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings("key-b")},
	}
	if err := SaveConfig(path, existing); err != nil {
		t.Fatal(err)
	}

	fresh := DefaultConfig()
	fresh.Channels = ChannelsConfig{}
	fresh.ModelList = SecureModelList{
		{ModelName: "gpt-4o", Model: "openai/gpt-4o"},
		{ModelName: "gpt-4o", Model: "openai/gpt-4o"},
	}
	if err := fresh.SecurityCopyFrom(path); err != nil {
		t.Fatal(err)
	}
	if got := fresh.ModelList[0].APIKey(); got != "key-a" {
		t.Fatalf("copied first key = %q, want key-a", got)
	}
	if got := fresh.ModelList[1].APIKey(); got != "key-b" {
		t.Fatalf("copied second key = %q, want key-b", got)
	}
}
