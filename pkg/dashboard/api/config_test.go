package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
)

func TestHandlePatchConfig_PreservesTurnProfile(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.Defaults.TurnProfile = config.TurnProfileConfig{
		Enabled:      true,
		History:      config.TurnProfileBlock{Mode: config.TurnProfileModeOff},
		SystemPrompt: config.TurnProfileBlock{Mode: config.TurnProfileModeOff},
		Skills:       config.TurnProfileBlock{Mode: config.TurnProfileModeOff},
		Tools: config.TurnProfileBlock{
			Mode:  config.TurnProfileModeCustom,
			Allow: []string{"web_search", "web_fetch"},
		},
	}
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"agents": {
			"defaults": {
				"max_tokens": 1234
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(updated) error = %v", err)
	}
	profile := updated.Agents.Defaults.TurnProfile
	if profile.Tools.Mode != config.TurnProfileModeCustom ||
		strings.Join(profile.Tools.Allow, ",") != "web_search,web_fetch" {
		t.Fatalf("profile tools = %#v, want custom web_search/web_fetch", profile.Tools)
	}
}

func TestHandlePatchConfig_RejectsInvalidTurnProfile(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"agents": {
			"defaults": {
				"turn_profile": {
					"enabled": true,
					"history": { "mode": "custom" }
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d, body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
	if !strings.Contains(rec.Body.String(), "history.mode custom is not supported") {
		t.Fatalf("body=%s, want turn profile validation error", rec.Body.String())
	}

	if _, err := config.LoadConfig(configPath); err != nil {
		t.Fatalf("LoadConfig() after rejected patch error = %v", err)
	}
}

func assertGatewayLogLevelApplied(t *testing.T, method, body string, want logger.LogLevel) {
	t.Helper()

	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	initialLevel := logger.GetLevel()
	logger.SetLevel(logger.INFO)
	t.Cleanup(func() {
		logger.SetLevel(initialLevel)
	})

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(method, "/api/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"%s /api/config status = %d, want %d, body=%s",
			method,
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}
	if got := logger.GetLevel(); got != want {
		t.Fatalf("logger.GetLevel() = %v, want %v", got, want)
	}
}

func TestHandleUpdateConfig_PreservesExecAllowRemoteDefaultWhenOmitted(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{
"version": 4,
		"agents": {
			"defaults": {
				"workspace": "~/.mole/workspace"
			}
		},
		"model_list": [
			{
				"model_name": "custom-default",
				"model": "openai/gpt-4o",
				"api_keys": ["sk-default"]
			}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.Tools.Exec.AllowRemote {
		t.Fatal("tools.exec.allow_remote should remain true when omitted from PUT /api/config")
	}
}

func TestHandleUpdateConfig_DoesNotInheritDefaultModelFields(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{
		"agents": {
			"defaults": {
				"workspace": "~/.mole/workspace"
			}
		},
		"model_list": [
			{
				"model_name": "custom-default",
				"model": "openai/gpt-4o",
				"api_key": "sk-default"
			}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.ModelList[0].APIBase; got != "" {
		t.Fatalf("model_list[0].api_base = %q, want empty string", got)
	}
}

func TestHandlePatchConfig_RejectsInvalidExecRegexPatterns(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"tools": {
			"exec": {
				"custom_deny_patterns": ["("]
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d, body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("custom_deny_patterns")) {
		t.Fatalf(
			"expected validation error mentioning custom_deny_patterns, body=%s",
			rec.Body.String(),
		)
	}
}

func TestHandlePatchConfig_AllowsInvalidExecRegexPatternsWhenExecDisabled(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"tools": {
			"exec": {
				"enabled": false,
				"custom_deny_patterns": ["("],
				"custom_allow_patterns": ["("]
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandlePatchConfig_SavesChannelListSettingsPatch(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"feishu": {
				"enabled": true,
				"allow_from": ["ou_patch_user"],
				"settings": {
					"app_id": "cli_patch_app",
					"app_secret": "patch-secret",
					"is_lark": true
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := cfg.Channels[config.ChannelFeishu]
	if !bc.Enabled {
		t.Fatal("feishu should be enabled after PATCH")
	}
	if len(bc.AllowFrom) != 1 || bc.AllowFrom[0] != "ou_patch_user" {
		t.Fatalf("feishu allow_from = %#v, want [\"ou_patch_user\"]", bc.AllowFrom)
	}
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	feishuCfg := decoded.(*config.FeishuSettings)
	if got := feishuCfg.AppID; got != "cli_patch_app" {
		t.Fatalf("feishu app_id = %q, want %q", got, "cli_patch_app")
	}
	if got := feishuCfg.AppSecret.String(); got != "patch-secret" {
		t.Fatalf("feishu app_secret = %q, want %q", got, "patch-secret")
	}
	if !feishuCfg.IsLark {
		t.Fatal("feishu is_lark should be true after PATCH")
	}
}

func TestHandlePatchConfig_NormalizesStringChannelArrayFields(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"webchat": {
				"type": "webchat",
				"allow_from": " ou_a\u200b，\u2060ou_b\tou_c\u202e，ou_a ",
				"group_trigger": {
					"prefixes": "/，!;\n?，/"
				},
				"settings": {
					"allow_origins": "https://a.example.com，http://localhost:5173，https://a.example.com"
				}
			},
			"feishu": {
				"type": "feishu",
				"settings": {
					"random_reaction_emoji": "THUMBSUP，OK\tEYES，THUMBSUP"
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	webchatChannel := cfg.Channels[config.ChannelWebChat]
	if len(webchatChannel.AllowFrom) != 3 ||
		webchatChannel.AllowFrom[0] != "ou_a" ||
		webchatChannel.AllowFrom[1] != "ou_b" ||
		webchatChannel.AllowFrom[2] != "ou_c" {
		t.Fatalf(
			"webchat allow_from = %#v, want [\"ou_a\", \"ou_b\", \"ou_c\"]",
			webchatChannel.AllowFrom,
		)
	}
	if len(webchatChannel.GroupTrigger.Prefixes) != 3 ||
		webchatChannel.GroupTrigger.Prefixes[0] != "/" ||
		webchatChannel.GroupTrigger.Prefixes[1] != "!;" ||
		webchatChannel.GroupTrigger.Prefixes[2] != "?" {
		t.Fatalf(
			"webchat group_trigger.prefixes = %#v, want [\"/\", \"!;\", \"?\"]",
			webchatChannel.GroupTrigger.Prefixes,
		)
	}

	decoded, err := webchatChannel.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() webchat error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if len(webchatCfg.AllowOrigins) != 2 ||
		webchatCfg.AllowOrigins[0] != "https://a.example.com" ||
		webchatCfg.AllowOrigins[1] != "http://localhost:5173" {
		t.Fatalf(
			"webchat allow_origins = %#v, want [\"https://a.example.com\", \"http://localhost:5173\"]",
			webchatCfg.AllowOrigins,
		)
	}

	feishuChannel := cfg.Channels[config.ChannelFeishu]
	decoded, err = feishuChannel.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() feishu error = %v", err)
	}
	feishuCfg := decoded.(*config.FeishuSettings)
	if len(feishuCfg.RandomReactionEmoji) != 3 ||
		feishuCfg.RandomReactionEmoji[0] != "THUMBSUP" ||
		feishuCfg.RandomReactionEmoji[1] != "OK" ||
		feishuCfg.RandomReactionEmoji[2] != "EYES" {
		t.Fatalf(
			"feishu random_reaction_emoji = %#v, want [\"THUMBSUP\", \"OK\", \"EYES\"]",
			feishuCfg.RandomReactionEmoji,
		)
	}
}

func TestHandlePatchConfig_NormalizesSingleNumericAllowFrom(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"telegram": {
				"type": "telegram",
				"allow_from": 123456
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	telegramChannel := cfg.Channels[config.ChannelTelegram]
	if len(telegramChannel.AllowFrom) != 1 || telegramChannel.AllowFrom[0] != "123456" {
		t.Fatalf("telegram allow_from = %#v, want [\"123456\"]", telegramChannel.AllowFrom)
	}
}

func TestHandlePatchConfig_RejectsInvalidChannelArrayFields(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	telegramChannel := cfg.Channels[config.ChannelTelegram]
	telegramChannel.AllowFrom = config.FlexibleStringSlice{"existing-user"}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	tests := []struct {
		name string
		body string
	}{
		{
			name: "object allow_from",
			body: `{
				"channel_list": {
					"telegram": {
						"type": "telegram",
						"allow_from": {"id": "bad"}
					}
				}
			}`,
		},
		{
			name: "boolean allow_from",
			body: `{
				"channel_list": {
					"telegram": {
						"type": "telegram",
						"allow_from": true
					}
				}
			}`,
		},
		{
			name: "object settings array",
			body: `{
				"channel_list": {
					"feishu": {
						"type": "feishu",
						"settings": {
							"random_reaction_emoji": {"name": "THUMBSUP"}
						}
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(configPath)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux)

			req := httptest.NewRequest(
				http.MethodPatch,
				"/api/config",
				bytes.NewBufferString(tt.body),
			)
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"PATCH /api/config status = %d, want %d, body=%s",
					rec.Code,
					http.StatusBadRequest,
					rec.Body.String(),
				)
			}

			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			telegramChannel := cfg.Channels[config.ChannelTelegram]
			if len(telegramChannel.AllowFrom) != 1 ||
				telegramChannel.AllowFrom[0] != "existing-user" {
				t.Fatalf(
					"telegram allow_from = %#v, want unchanged [\"existing-user\"]",
					telegramChannel.AllowFrom,
				)
			}
		})
	}
}

func TestHandlePatchConfig_RejectsNegativeStreamingDeliveryValues(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"webchat": {
				"settings": {
					"streaming": {
						"enabled": true,
						"throttle_seconds": -1
					}
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
	if !strings.Contains(rec.Body.String(), "streaming.throttle_seconds") {
		t.Fatalf(
			"response body = %q, want streaming.throttle_seconds validation error",
			rec.Body.String(),
		)
	}
}

func TestHandlePatchConfig_ClearingAllowFromDoesNotLeaveEmptyStringItem(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	feishuChannel := cfg.Channels[config.ChannelFeishu]
	feishuChannel.Enabled = true
	feishuChannel.AllowFrom = config.FlexibleStringSlice{"ou_existing_user"}
	decoded, err := feishuChannel.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	feishuCfg := decoded.(*config.FeishuSettings)
	feishuCfg.AppID = "cli_existing_app"
	feishuCfg.AppSecret = *config.NewSecureString("existing-secret")
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"feishu": {
				"enabled": false,
				"allow_from": "",
				"settings": {
					"app_id": "cli_existing_app"
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	feishuChannel = cfg.Channels[config.ChannelFeishu]
	if len(feishuChannel.AllowFrom) != 0 {
		t.Fatalf("feishu allow_from = %#v, want empty slice", feishuChannel.AllowFrom)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	if strings.Contains(string(configData), `"allow_from": [""]`) {
		t.Fatalf(
			"config file should not contain empty-string allow_from item: %s",
			string(configData),
		)
	}
}

func TestHandlePatchConfig_CreatesMissingChannelWithTypeAndSecret(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	delete(cfg.Channels, config.ChannelQQ)
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"channel_list": {
			"qq": {
				"enabled": true,
				"allow_from": ["*"],
				"type": "qq",
				"settings": {
					"app_id": "102000000",
					"app_secret": "qq-patch-password"
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := cfg.Channels[config.ChannelQQ]
	if bc == nil {
		t.Fatal("qq channel should exist after PATCH")
	}
	if got := bc.Type; got != config.ChannelQQ {
		t.Fatalf("qq type = %q, want %q", got, config.ChannelQQ)
	}
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	qqCfg := decoded.(*config.QQSettings)
	if got := qqCfg.AppID; got != "102000000" {
		t.Fatalf("qq app_id = %q, want %q", got, "102000000")
	}
	if got := qqCfg.AppSecret.String(); got != "qq-patch-password" {
		t.Fatalf("qq app_secret = %q, want %q", got, "qq-patch-password")
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	if bytes.Contains(configData, []byte("qq-patch-password")) {
		t.Fatalf("config file leaked qq app_secret: %s", string(configData))
	}
}

// setupWebChatEnabledEnv creates a test environment with WebChat channel enabled and
// its token promoted to .env while config.json keeps a $VAR reference.
func setupWebChatEnabledEnv(t *testing.T) (string, func()) {
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
	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	bc.Enabled = true
	bc.AllowFrom = config.FlexibleStringSlice{"*"}
	webchatCfg.Token = *config.NewSecureString("test-webchat-token")

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

func TestHandleUpdateConfig_SucceedsWhenWebChatTokenInEnvOnly(t *testing.T) {
	configPath, cleanup := setupWebChatEnabledEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// PUT request with webchat enabled but no token in JSON — the token is referenced by .env
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{
		"version": 1,
		"agents": {
			"defaults": {
				"workspace": "~/.mole/workspace",
				"model_name": "custom-default"
			}
		},
		"channels": {
			"webchat": {
				"enabled": true,
				"ping_interval": 30,
				"read_timeout": 60,
				"max_connections": 100
			}
		},
		"model_list": [
			{
				"model_name": "custom-default",
				"model": "openai/gpt-4o",
				"api_keys": ["sk-default"]
			}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PUT /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}
}

func TestHandlePatchConfig_SucceedsWhenWebChatTokenInEnvOnly(t *testing.T) {
	configPath, cleanup := setupWebChatEnabledEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// PATCH request changing an unrelated field — the webchat token still resolves from .env
	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"gateway": {
			"log_level": "info"
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}
}

func TestHandleUpdateConfig_AppliesGatewayLogLevel(t *testing.T) {
	assertGatewayLogLevelApplied(t, http.MethodPut, `{
		"version": 1,
		"agents": {
			"defaults": {
				"workspace": "~/.mole/workspace",
				"model_name": "custom-default"
			}
		},
		"gateway": {
			"log_level": "error"
		},
		"model_list": [
			{
				"model_name": "custom-default",
				"model": "openai/gpt-4o",
				"api_keys": ["sk-default"]
			}
		]
	}`, logger.ERROR)
}

func TestHandlePatchConfig_AppliesGatewayLogLevel(t *testing.T) {
	assertGatewayLogLevelApplied(t, http.MethodPatch, `{
		"gateway": {
			"log_level": "debug"
		}
	}`, logger.DEBUG)
}

func TestHandlePatchConfig_PreservesDebugFlagOverride(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	initialLevel := logger.GetLevel()
	logger.SetLevel(logger.INFO)
	t.Cleanup(func() {
		logger.SetLevel(initialLevel)
	})

	h := NewHandler(configPath)
	h.SetDebug(true)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"gateway": {
			"log_level": "error"
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}
	if got := logger.GetLevel(); got != logger.DEBUG {
		t.Fatalf("logger.GetLevel() = %v, want %v", got, logger.DEBUG)
	}
}

func TestHandlePatchConfig_DoesNotPersistShadowRegistryAuthTokenField(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"tools": {
			"skills": {
				"registries": {
					"github": {
						"_auth_token": "ghp-shadow-token"
					}
				}
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"PATCH /api/config status = %d, want %d, body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	githubRegistry, ok := cfg.Tools.Skills.Registries.Get("github")
	if !ok {
		t.Fatal("github registry missing after PATCH")
	}
	if got := githubRegistry.AuthToken.String(); got != "ghp-shadow-token" {
		t.Fatalf("github registry auth token = %q, want %q", got, "ghp-shadow-token")
	}
	if got := githubRegistry.BaseURL; got != "https://github.com" {
		t.Fatalf("github registry base_url = %q, want %q", got, "https://github.com")
	}

	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(configPath) error = %v", err)
	}
	if strings.Contains(string(rawConfig), "_auth_token") {
		t.Fatalf(
			"config.json should not persist _auth_token shadow field, got:\n%s",
			string(rawConfig),
		)
	}
}

func TestHandlePatchConfig_PreservesRegistryAuthTokenReference(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	t.Setenv("GITHUB_TOKEN", "real")
	envPath := config.EnvFilePath(configPath)
	if err := os.WriteFile(envPath, []byte("GITHUB_TOKEN=real\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	registry, ok := cfg.Tools.Skills.Registries.Get("github")
	if !ok {
		t.Fatal("default config missing github registry")
	}
	registry.AuthToken = *config.NewSecureString("$GITHUB_TOKEN")
	cfg.Tools.Skills.Registries.Set("github", registry)
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	initialLevel := logger.GetLevel()
	t.Cleanup(func() { logger.SetLevel(initialLevel) })
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"gateway": {"log_level": "debug"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(rawConfig), `"auth_token": "$GITHUB_TOKEN"`) {
		t.Fatalf("config lost the auth_token reference:\n%s", rawConfig)
	}
	if strings.Contains(string(rawConfig), `"auth_token": "real"`) {
		t.Fatalf("config stored the resolved token:\n%s", rawConfig)
	}
	rawEnv, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(.env) error = %v", err)
	}
	if !strings.Contains(string(rawEnv), "GITHUB_TOKEN=real") {
		t.Fatalf(".env lost the real token:\n%s", rawEnv)
	}
	if strings.Contains(string(rawEnv), "GITHUB_TOKEN=$GITHUB_TOKEN") {
		t.Fatalf(".env got a self-referential token:\n%s", rawEnv)
	}

	reloaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("reload LoadConfig() error = %v", err)
	}
	reloadedRegistry, ok := reloaded.Tools.Skills.Registries.Get("github")
	if !ok {
		t.Fatal("github registry missing after reload")
	}
	if got := reloadedRegistry.AuthToken.String(); got != "real" {
		t.Fatalf("reloaded auth token = %q, want real", got)
	}
}

func TestHandlePatchConfig_ReadsShadowRegistryTokenWithoutChannelList(t *testing.T) {
	cfg := config.DefaultConfig()
	registry, ok := cfg.Tools.Skills.Registries.Get("github")
	if !ok {
		t.Fatal("default config missing github registry")
	}
	registry.AuthToken = *config.NewSecureString("old")
	cfg.Tools.Skills.Registries.Set("github", registry)

	applyConfigSecretsFromMap(cfg, map[string]any{
		"tools": map[string]any{
			"skills": map[string]any{
				"registries": map[string]any{
					"github": map[string]any{
						"auth_token":  "stale",
						"_auth_token": "$GITHUB_TOKEN",
					},
				},
			},
		},
	})

	updated, _ := cfg.Tools.Skills.Registries.Get("github")
	data, err := json.Marshal(updated.AuthToken)
	if err != nil {
		t.Fatalf("marshal auth token: %v", err)
	}
	if string(data) != `"$GITHUB_TOKEN"` {
		t.Fatalf("auth_token = %s, want the $GITHUB_TOKEN reference", data)
	}
}

func TestHandlePatchConfig_ReadsShadowRegistryTokenInListForm(t *testing.T) {
	cfg := config.DefaultConfig()
	registry, ok := cfg.Tools.Skills.Registries.Get("github")
	if !ok {
		t.Fatal("default config missing github registry")
	}
	registry.AuthToken = *config.NewSecureString("old")
	cfg.Tools.Skills.Registries.Set("github", registry)

	applyConfigSecretsFromMap(cfg, map[string]any{
		"tools": map[string]any{
			"skills": map[string]any{
				"registries": []any{
					map[string]any{
						"name":        "github",
						"auth_token":  "$GITHUB_TOKEN",
						"_auth_token": "$GITHUB_TOKEN",
					},
				},
			},
		},
	})

	updated, _ := cfg.Tools.Skills.Registries.Get("github")
	data, err := json.Marshal(updated.AuthToken)
	if err != nil {
		t.Fatalf("marshal auth token: %v", err)
	}
	if string(data) != `"$GITHUB_TOKEN"` {
		t.Fatalf("auth_token = %s, want the $GITHUB_TOKEN reference", data)
	}
}

func TestHandlePatchConfig_AllowsInvalidDenyRegexPatternsWhenDenyPatternsDisabled(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"tools": {
			"exec": {
				"enabled": true,
				"enable_deny_patterns": false,
				"custom_deny_patterns": ["("]
			}
		}
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// testCommandPatterns is a helper that sets up a handler and sends a test-command-patterns request.
func testCommandPatterns(t *testing.T, configPath string, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/config/test-command-patterns",
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleTestCommandPatterns_MatchesWhitelist(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": ["^echo\\s+hello"],
		"deny_patterns": ["^rm\\s+-rf"],
		"command": "echo hello world"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf("expected allowed=true, body=%s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("expected blocked=false when whitelist matches, body=%s", rec.Body.String())
	}
}

func TestHandleTestCommandPatterns_MatchesBlacklistNotWhitelist(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": ["^echo\\s+hello"],
		"deny_patterns": ["^rm\\s+-rf"],
		"command": "rm -rf /tmp"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("expected blocked=true, body=%s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf(
			"expected allowed=false when blacklist matches but not whitelist, body=%s",
			rec.Body.String(),
		)
	}
}

func TestHandleTestCommandPatterns_MatchesNeither(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": ["^echo\\s+hello"],
		"deny_patterns": ["^rm\\s+-rf"],
		"command": "ls -la"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf("expected allowed=false, body=%s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("expected blocked=false, body=%s", rec.Body.String())
	}
}

func TestHandleTestCommandPatterns_CaseInsensitiveWithGoFlag(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": ["(?i)^ECHO"],
		"deny_patterns": [],
		"command": "echo hello"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf("expected allowed=true with Go (?i) flag, body=%s", rec.Body.String())
	}
}

func TestHandleTestCommandPatterns_EmptyPatterns(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": [],
		"deny_patterns": [],
		"command": "rm -rf /tmp"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf("expected allowed=false with empty patterns, body=%s", rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("expected blocked=false with empty patterns, body=%s", rec.Body.String())
	}
}

func TestHandleTestCommandPatterns_InvalidRegexSkipped(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": ["([[", "^echo"],
		"deny_patterns": [],
		"command": "echo hello"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf(
			"expected allowed=true, invalid pattern skipped and valid one matched, body=%s",
			rec.Body.String(),
		)
	}
}

func TestHandleTestCommandPatterns_ReturnsMatchedPattern(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	rec := testCommandPatterns(t, configPath, `{
		"allow_patterns": [],
		"deny_patterns": ["\\$(?i)[a-zA-Z_]*(SECRET|KEY|PASSWORD|TOKEN|AUTH)[a-zA-Z0-9_]*"],
		"command": "echo $GITHUB_API_KEY"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"blocked":true`)) {
		t.Fatalf("expected blocked=true, body=%s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`matched_blacklist`)) {
		t.Fatalf("expected matched_blacklist field, body=%s", rec.Body.String())
	}
}

func TestHandleTestCommandPatterns_InvalidJSON(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/config/test-command-patterns",
		bytes.NewBufferString(`{invalid json}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d, body=%s",
			rec.Code,
			http.StatusBadRequest,
			rec.Body.String(),
		)
	}
}

func TestApplyConfigSecretsFromMap_TelegramToken(t *testing.T) {
	cfg := config.DefaultConfig()
	bc := cfg.Channels["telegram"]
	bc.Enabled = true
	// Pre-decode so extend is populated
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	tgCfg := decoded.(*config.TelegramSettings)
	tgCfg.Token = *config.NewSecureString("original-token")

	raw := map[string]any{
		"channel_list": map[string]any{
			"telegram": map[string]any{
				"enabled": true,
				"token":   "secret-from-api",
			},
		},
	}

	applyConfigSecretsFromMap(cfg, raw)

	if got := tgCfg.Token.String(); got != "secret-from-api" {
		t.Fatalf("telegram token = %q, want %q", got, "secret-from-api")
	}
}

func TestApplyConfigSecretsFromMap_MultipleChannels(t *testing.T) {
	cfg := config.DefaultConfig()

	// Setup telegram
	bc := cfg.Channels["telegram"]
	bc.Enabled = true
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() telegram error = %v", err)
	}
	tgCfg := decoded.(*config.TelegramSettings)
	tgCfg.Token = *config.NewSecureString("old-telegram-token")

	// Setup feishu
	bc = cfg.Channels["feishu"]
	bc.Enabled = true
	decoded, err = bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() feishu error = %v", err)
	}
	feishuCfg := decoded.(*config.FeishuSettings)
	feishuCfg.AppSecret = *config.NewSecureString("old-feishu-app-secret")

	raw := map[string]any{
		"channel_list": map[string]any{
			"telegram": map[string]any{
				"enabled": true,
				"settings": map[string]any{
					"token": "new-telegram-token",
				},
			},
			"feishu": map[string]any{
				"enabled": true,
				"settings": map[string]any{
					"app_secret": "new-feishu-app-secret",
				},
			},
		},
	}

	applyConfigSecretsFromMap(cfg, raw)

	if got := tgCfg.Token.String(); got != "new-telegram-token" {
		t.Fatalf("telegram token = %q, want %q", got, "new-telegram-token")
	}
	if got := feishuCfg.AppSecret.String(); got != "new-feishu-app-secret" {
		t.Fatalf("feishu app_secret = %q, want %q", got, "new-feishu-app-secret")
	}
}

func TestApplyConfigSecretsFromMap_SkipsNonStringValues(t *testing.T) {
	cfg := config.DefaultConfig()
	bc := cfg.Channels["telegram"]
	bc.Enabled = true
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	tgCfg := decoded.(*config.TelegramSettings)
	tgCfg.Token = *config.NewSecureString("original-token")

	raw := map[string]any{
		"channel_list": map[string]any{
			"telegram": map[string]any{
				"enabled": true,
				"token":   12345, // not a string, should be skipped
			},
		},
	}

	applyConfigSecretsFromMap(cfg, raw)

	if got := tgCfg.Token.String(); got != "original-token" {
		t.Fatalf("telegram token = %q, want %q", got, "original-token")
	}
}

func TestApplyConfigSecretsFromMap_ChannelNotDecodedYet(t *testing.T) {
	cfg := config.DefaultConfig()
	bc := cfg.Channels["telegram"]
	bc.Enabled = true
	// Don't decode — let the function handle lazy decoding
	bc.Type = config.ChannelTelegram

	raw := map[string]any{
		"channel_list": map[string]any{
			"telegram": map[string]any{
				"enabled": true,
				"token":   "lazy-decoded-token",
			},
		},
	}

	applyConfigSecretsFromMap(cfg, raw)

	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	tgCfg := decoded.(*config.TelegramSettings)
	if got := tgCfg.Token.String(); got != "lazy-decoded-token" {
		t.Fatalf("telegram token = %q, want %q", got, "lazy-decoded-token")
	}
}

func TestHandlePatchConfig_PreservesDashboardToken(t *testing.T) {
	// A console config save must not drop the credential that gates the console:
	// losing it would silently re-open /setup to anyone who can reach the port.
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Dashboard.Token = *config.NewSecureString("console-token")
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"agents": {"defaults": {"max_tokens": 1234}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(updated) error = %v", err)
	}
	if got := updated.Dashboard.Token.String(); got != "console-token" {
		t.Fatalf("dashboard token = %q, want it preserved across a config save", got)
	}
}

func TestHandlePatchConfig_RejectsEmptyAllowFromForEnabledChannel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	patch := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(`{"channel_list": {"feishu": {"enabled": true}}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("enabling a channel without allow_from = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "allow_from") {
		t.Fatalf("body = %s, want an allow_from message", rec.Body.String())
	}

	// The explicit form is what "allow everyone" looks like.
	rec = patch(`{"channel_list": {"feishu": {"enabled": true, "allow_from": ["*"]}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("explicit allow_from = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	// A disabled channel may stay unconfigured.
	rec = patch(`{"channel_list": {"weixin": {"enabled": false}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disabled channel = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateConfig_PreservesOmittedAllowRemote(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Exec.AllowRemote = false
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// A PUT body that never mentions tools.exec must not flip the stored value
	// back to the default.
	current, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	payload, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var body map[string]any
	if err = json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	tools, _ := body["tools"].(map[string]any)
	delete(tools, "exec")
	body["tools"] = tools
	trimmed, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal(patch) error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(trimmed))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(updated) error = %v", err)
	}
	if updated.Tools.Exec.AllowRemote {
		t.Fatal("allow_remote was reset to the default instead of preserved")
	}
}

func TestPatchConfigRejectsABadApprovalPolicy(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"tools":{"approval":{"rules":[{"tool":"exec","action":"maybe"}]}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rules[0].action") {
		t.Fatalf("body = %s, want the offending field", rec.Body.String())
	}
	// The file must not have been written.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Tools.Approval.Rules) != 0 {
		t.Fatalf("rules = %+v, want the save to be rejected", cfg.Tools.Approval.Rules)
	}
}

func TestHandleUpdateConfigPromotesSecretToEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.ModelList = config.SecureModelList{}
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(path)
	body := `{"version":4,"channel_list":{"telegram":{"enabled":true,"type":"telegram","allow_from":["*"],"settings":{"token":"tg-secret"}}},"model_list":[],"agents":{"defaults":{"model_name":""}}}`
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleUpdateConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	rawConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(rawConfig), `"$TELEGRAM_TOKEN"`) {
		t.Fatalf("config missing ref:\n%s", rawConfig)
	}
	if strings.Contains(string(rawConfig), "tg-secret") {
		t.Fatalf("config leaked secret:\n%s", rawConfig)
	}
	rawEnv, err := os.ReadFile(config.EnvFilePath(path))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if !strings.Contains(string(rawEnv), "TELEGRAM_TOKEN=tg-secret") {
		t.Fatalf(".env missing token:\n%s", rawEnv)
	}
}

func TestHandleGetConfig_RedactsPlaintextSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := config.DefaultConfig()
	cfg.ModelList = config.SecureModelList{{
		ModelName: "redact-model",
		Model:     "openai/gpt-4o",
		APIKeys:   config.SimpleSecureStrings("sk-plain-secret"),
	}}
	cfg.Agents.Defaults.ModelName = "redact-model"
	cfg.Dashboard.Token = *config.NewSecureString("dash-plain-secret")
	telegram := cfg.Channels["telegram"]
	decoded, err := telegram.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	decoded.(*config.TelegramSettings).Token = *config.NewSecureString("tg-plain-secret")
	telegram.Enabled = true

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if !bytes.Contains(data, []byte("dash-plain-secret")) {
		t.Fatalf("test setup should write plaintext secrets:\n%s", data)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	h := NewHandler(path)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{"dash-plain-secret", "sk-plain-secret", "tg-plain-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("GET /api/config leaked %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "redact-model") {
		t.Fatalf("GET /api/config dropped non-secret settings: %s", body)
	}
}

func TestHandleGetConfig_KeepsSecretReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"version": 4,
		"agents": {"defaults": {"model_name": "ref-model"}},
		"model_list": [{"model_name": "ref-model", "model": "openai/gpt-4o", "api_keys": ["$PLAIN_MODEL_KEY"]}],
		"dashboard": {"token": "$DASHBOARD_TOKEN"}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("PLAIN_MODEL_KEY", "resolved-model-secret")
	t.Setenv("DASHBOARD_TOKEN", "resolved-dashboard-secret")

	h := NewHandler(path)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "resolved-model-secret") || strings.Contains(body, "resolved-dashboard-secret") {
		t.Fatalf("GET /api/config leaked a resolved secret: %s", body)
	}
	if !strings.Contains(body, "$PLAIN_MODEL_KEY") || !strings.Contains(body, "$DASHBOARD_TOKEN") {
		t.Fatalf("GET /api/config dropped references: %s", body)
	}
}

func writeConfigWithPlainRef(t *testing.T, path string) {
	t.Helper()
	data := `{
		"version": 4,
		"agents": {"defaults": {"model_name": "ref-model"}},
		"model_list": [{"model_name": "ref-model", "model": "openai/gpt-4o", "api_base": "$TEST_API_BASE"}]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestHandleGetConfig_KeepsPlainEnvRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfigWithPlainRef(t, path)
	t.Setenv("TEST_API_BASE", "https://resolved.test/v1")

	h := NewHandler(path)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"api_base":"$TEST_API_BASE"`) &&
		!strings.Contains(body, `"api_base": "$TEST_API_BASE"`) {
		t.Fatalf("GET /api/config lost the reference: %s", body)
	}
	if strings.Contains(body, "https://resolved.test/v1") {
		t.Fatalf("GET /api/config returned the resolved value: %s", body)
	}
}

func TestHandlePatchConfig_PreservesPlainEnvRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfigWithPlainRef(t, path)
	t.Setenv("TEST_API_BASE", "https://resolved.test/v1")

	initialLevel := logger.GetLevel()
	t.Cleanup(func() { logger.SetLevel(initialLevel) })
	h := NewHandler(path)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"gateway": {"log_level": "debug"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	rawConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(rawConfig), `"api_base": "$TEST_API_BASE"`) {
		t.Fatalf("PATCH baked the resolved value into config:\n%s", rawConfig)
	}
	reloaded, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("reload LoadConfig() error = %v", err)
	}
	if got := reloaded.ModelList[0].APIBase; got != "https://resolved.test/v1" {
		t.Fatalf("reloaded APIBase = %q", got)
	}
}

func TestRouterDoesNotRegisterAutostart(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "config.json"))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/autostart", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/system/autostart status = %d, want 404", rec.Code)
	}
}
