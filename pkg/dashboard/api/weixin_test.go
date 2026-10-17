package api

import (
	"path/filepath"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestSaveWeixinBindingPersistsBindingAndRequestsReload(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	reloads := 0
	h.SetRuntime(&Runtime{Address: "127.0.0.1:18790", Reload: func() { reloads++ }})

	if err := h.saveWeixinBinding("bot-token", "bot-account"); err != nil {
		t.Fatalf("saveWeixinBinding() error = %v", err)
	}

	savedCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := savedCfg.Channels["weixin"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	wxCfg := decoded.(*config.WeixinSettings)
	if got := wxCfg.Token.String(); got != "bot-token" {
		t.Fatalf("Weixin.Token() = %q, want %q", got, "bot-token")
	}
	if got := wxCfg.AccountID; got != "bot-account" {
		t.Fatalf("Weixin.AccountID = %q, want %q", got, "bot-account")
	}
	if !bc.Enabled {
		t.Fatalf("Weixin.Enabled = false, want true")
	}
	if reloads != 1 {
		t.Fatalf("reload requests = %d, want 1: the running channel must pick the binding up", reloads)
	}
}
