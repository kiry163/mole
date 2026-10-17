// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	svc "github.com/kardianos/service"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/pid"
)

func TestURL(t *testing.T) {
	tests := []struct {
		host  string
		port  int
		setup bool
		want  string
	}{
		{"127.0.0.1", 18790, false, "http://127.0.0.1:18790"},
		{"0.0.0.0", 18790, true, "http://127.0.0.1:18790/setup"},
		{"::", 18790, false, "http://127.0.0.1:18790"},
		{"", 18790, false, "http://127.0.0.1:18790"},
		{"[::]", 18790, true, "http://127.0.0.1:18790/setup"},
		{"::1", 18790, false, "http://[::1]:18790"},
		{"[::1]", 18790, false, "http://[::1]:18790"},
		{"localhost", 9000, false, "http://localhost:9000"},
	}
	for _, tt := range tests {
		if got := URL(tt.host, tt.port, tt.setup); got != tt.want {
			t.Errorf("URL(%q,%d,%t) = %q, want %q", tt.host, tt.port, tt.setup, got, tt.want)
		}
	}
}

func TestCollectStatusManualRunning(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	defer pid.RemovePidFile(home)

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	st, err := (&Manager{Spec: spec}).CollectStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !st.ManualRunning {
		t.Fatalf("manual running = false, status = %+v", st)
	}
	if !st.Running {
		t.Fatalf("running = false, status = %+v", st)
	}
	if st.Installed {
		t.Fatalf("installed = true without a service manager, status = %+v", st)
	}
	if st.PID != os.Getpid() {
		t.Fatalf("PID = %d, want %d", st.PID, os.Getpid())
	}
	if st.ConfigError != "" {
		t.Fatalf("config error = %q", st.ConfigError)
	}
	if !strings.HasSuffix(st.URL, "/setup") {
		t.Fatalf("uninitialized URL = %q, want /setup suffix", st.URL)
	}
}

func TestCollectStatusHealth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ready bool
		want  string
	}{
		{"ready", true, "ready"},
		{"degraded", false, "degraded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ready":
					if !tc.ready {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusOK)
				case "/health":
					w.WriteHeader(http.StatusOK)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			u, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				t.Fatal(err)
			}

			home := t.TempDir()
			configPath := filepath.Join(home, "config.json")
			cfg := config.DefaultConfig()
			cfg.Gateway.Host = "127.0.0.1"
			cfg.Gateway.Port = port
			if err := config.SaveConfig(configPath, cfg); err != nil {
				t.Fatal(err)
			}
			if _, err := pid.WritePidFile(home, "127.0.0.1", port); err != nil {
				t.Fatal(err)
			}
			defer pid.RemovePidFile(home)

			spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
			st, err := (&Manager{Spec: spec}).CollectStatus()
			if err != nil {
				t.Fatal(err)
			}
			if st.Health != tc.want {
				t.Fatalf("health = %q, want %q (status = %+v)", st.Health, tc.want, st)
			}
		})
	}
}

func TestCollectStatusConfigError(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version": 3,`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	defer pid.RemovePidFile(home)

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	st, err := (&Manager{Spec: spec}).CollectStatus()
	if err != nil {
		t.Fatalf("CollectStatus() error = %v, want status with config error", err)
	}
	if st.ConfigError == "" {
		t.Fatal("config error was not recorded")
	}
	if st.PID != os.Getpid() {
		t.Fatalf("PID = %d, want %d", st.PID, os.Getpid())
	}
	if !st.ManualRunning {
		t.Fatalf("manual running = false, status = %+v", st)
	}
	if st.URL != "http://127.0.0.1:18790/setup" {
		t.Fatalf("URL = %q", st.URL)
	}
}

func TestCollectStatusEnvChanged(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	defer pid.RemovePidFile(home)

	envPath := config.EnvFilePath(configPath)
	if err := os.WriteFile(envPath, []byte("PATH=/bin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(pid.FilePath(home), old, old); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(envPath, now, now); err != nil {
		t.Fatal(err)
	}

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	st, err := (&Manager{Spec: spec}).CollectStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !st.EnvChanged {
		t.Fatalf("env changed = false, status = %+v", st)
	}
}

func TestCollectStatusNeverIncludesToken(t *testing.T) {
	const secret = "super-secret-token-value"
	t.Setenv("DASHBOARD_TOKEN", secret)

	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	cfg.Dashboard.Token = *config.NewSecureString("$DASHBOARD_TOKEN")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	defer pid.RemovePidFile(home)

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	st, err := (&Manager{Spec: spec}).CollectStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.ConfigError != "" {
		t.Fatalf("config error = %q", st.ConfigError)
	}
	if strings.HasSuffix(st.URL, "/setup") {
		t.Fatalf("initialized URL = %q, want no /setup suffix", st.URL)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("status JSON leaked the dashboard token: %s", raw)
	}
}

func TestWaitReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = port
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	if err := WaitReady(spec, time.Second); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
}

func TestWaitReadyTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = port
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	err = WaitReady(spec, 100*time.Millisecond)
	if err == nil {
		t.Fatal("WaitReady() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "ready") {
		t.Fatalf("WaitReady() error = %v, want readiness detail", err)
	}
}

// stubService lets tests control the kardianos Status result.
type stubService struct {
	svc.Service
	status svc.Status
	err    error
}

func (s stubService) Status() (svc.Status, error) { return s.status, s.err }

func TestCollectStatusNotInstalled(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	m := &Manager{
		Spec:    spec,
		Service: stubService{status: svc.StatusUnknown, err: svc.ErrNotInstalled},
	}
	st, err := m.CollectStatus()
	if err != nil {
		t.Fatalf("CollectStatus() error = %v, want nil for ErrNotInstalled", err)
	}
	if st.Installed {
		t.Fatalf("installed = true, status = %+v", st)
	}
	if st.Running || st.ManualRunning {
		t.Fatalf("running state set without a live process, status = %+v", st)
	}
	if st.ConfigError != "" {
		t.Fatalf("config error = %q", st.ConfigError)
	}
	if st.URL == "" {
		t.Fatal("URL is empty")
	}
}

func TestCollectStatusSetupWhenTokenReferenceEmpty(t *testing.T) {
	t.Setenv("DASHBOARD_TOKEN", "")

	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790
	cfg.Dashboard.Token = *config.NewSecureString("$DASHBOARD_TOKEN")
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	spec := Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/true", GOOS: "darwin"}
	st, err := (&Manager{Spec: spec}).CollectStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.ConfigError != "" {
		t.Fatalf("config error = %q", st.ConfigError)
	}
	if !strings.HasSuffix(st.URL, "/setup") {
		t.Fatalf("URL = %q, want /setup for an empty token reference", st.URL)
	}
}
