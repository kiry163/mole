package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kiry163/mole/pkg/channels"
	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/dashboard/api"
	"github.com/kiry163/mole/pkg/gateway"
)

// TestMountReloadsWhenWebchatWasJustEnabled covers the fresh-install path: New
// enables the webchat channel in the config file, but the gateway has already
// loaded the config by the time the console mounts, so Mount must ask the host
// for a reload or the channel never starts (and its /webchat/ws route is never
// registered on the shared mux).
func TestMountReloadsWhenWebchatWasJustEnabled(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "config.json")

	calls := 0
	env := &gateway.RuntimeEnv{
		Manager:       &channels.Manager{},
		Address:       "127.0.0.1:0",
		RequestReload: func() { calls++ },
	}
	if err := Mount(env, Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("Mount() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("RequestReload calls = %d, want 1", calls)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := cfg.Channels["webchat"]
	if bc == nil || !bc.Enabled {
		t.Fatalf("webchat channel not enabled after Mount: %+v", bc)
	}
}

// TestMountDoesNotReloadWhenWebchatAlreadyEnabled guards the steady-state
// start: when the config already has webchat enabled nothing changed, so the
// host must not be asked to reload.
func TestMountDoesNotReloadWhenWebchatAlreadyEnabled(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "config.json")

	// Seed a config that already carries an enabled webchat channel. This is the
	// state a second `mole serve` start finds on disk.
	h := api.NewHandler(configPath)
	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}
	if !changed {
		t.Fatal("seed expected EnsureWebChatChannel to change a fresh config")
	}

	calls := 0
	env := &gateway.RuntimeEnv{
		Manager:       &channels.Manager{},
		Address:       "127.0.0.1:0",
		RequestReload: func() { calls++ },
	}
	if err := Mount(env, Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("Mount() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("RequestReload calls = %d, want 0 (steady-state start must not reload)", calls)
	}
}

// TestMountExpandsWorkspaceTemplateOnFirstRun covers the fresh-install path: the
// console mount is the only place that has both the embedded template and the
// resolved workspace path, and it runs before the first request is served, so a
// workspace that does not exist yet must be materialised there.
func TestMountExpandsWorkspaceTemplateOnFirstRun(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "config.json")

	// The workspace is set explicitly instead of relying on the MOLE_HOME default,
	// so the test can never expand into a real user workspace whatever the
	// environment resolves to.
	cfg := config.DefaultConfig()
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace

	env := &gateway.RuntimeEnv{
		Manager: &channels.Manager{},
		Address: "127.0.0.1:0",
		Config:  cfg,
	}
	if err := Mount(env, Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	for _, name := range []string{"AGENT.md", "SOUL.md", "USER.md", "memory/MEMORY.md"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); err != nil {
			t.Fatalf("expected %s after Mount: %v", name, err)
		}
	}
	skills, err := filepath.Glob(filepath.Join(workspace, "skills", "*", "SKILL.md"))
	if err != nil || len(skills) == 0 {
		t.Fatalf("expected skills after Mount: %v (err=%v)", skills, err)
	}
}

// TestMountKeepsAnExistingWorkspace guards the guarantee that matters most: a
// second mount (a normal restart) must not touch a workspace the user has since
// edited.
func TestMountKeepsAnExistingWorkspace(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "config.json")

	cfg := config.DefaultConfig()
	workspace := filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.Workspace = workspace

	env := &gateway.RuntimeEnv{Manager: &channels.Manager{}, Address: "127.0.0.1:0", Config: cfg}
	if err := Mount(env, Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("first Mount() error = %v", err)
	}

	agentFile := filepath.Join(workspace, "AGENT.md")
	if _, err := os.Stat(agentFile); err != nil {
		t.Fatalf("first Mount did not create %s: %v", agentFile, err)
	}

	custom := []byte("my own agent definition\n")
	if err := os.WriteFile(agentFile, custom, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Mount(env, Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("second Mount() error = %v", err)
	}
	got, err := os.ReadFile(agentFile)
	if err != nil || string(got) != string(custom) {
		t.Fatalf("AGENT.md = %q, err = %v; a restart must not overwrite the user's file", got, err)
	}
}
