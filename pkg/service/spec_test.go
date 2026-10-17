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

func TestSpecNames(t *testing.T) {
	userHome, _ := os.UserHomeDir()
	home := filepath.Join(userHome, ".mole")
	custom := filepath.Join(t.TempDir(), "mole-dev")
	tests := []struct {
		goos string
		home string
		want string
	}{
		{"darwin", home, "io.mole.agent"},
		{"darwin", custom, "io.mole.agent." + shortHash(custom)},
		{"linux", home, "mole"},
		{"linux", custom, "mole-" + shortHash(custom)},
		{"windows", home, "Mole-" + shortHash(home)},
		{"windows", custom, "Mole-" + shortHash(custom)},
	}
	for _, tt := range tests {
		s := Spec{HomePath: tt.home, ConfigPath: filepath.Join(tt.home, "config.json"), GOOS: tt.goos}
		if got := s.Name(); got != tt.want {
			t.Errorf("%s %s Name() = %q, want %q", tt.goos, tt.home, got, tt.want)
		}
	}
}

func TestSpecDisplayName(t *testing.T) {
	userHome, _ := os.UserHomeDir()
	home := filepath.Join(userHome, ".mole")
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), GOOS: "darwin"}
	if got := s.DisplayName(); got != "Mole Agent" {
		t.Fatalf("default DisplayName = %q", got)
	}
	custom := filepath.Join(t.TempDir(), "mole-dev")
	s.HomePath = custom
	if got := s.DisplayName(); got != "Mole Agent ("+custom+")" {
		t.Fatalf("custom DisplayName = %q", got)
	}
}

func TestNewSpecRejectsRelativeBootstrap(t *testing.T) {
	t.Setenv("MOLE_HOME", "relative-home")
	if _, err := NewSpec("relative-home", ""); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative MOLE_HOME error = %v", err)
	}
	t.Setenv("MOLE_HOME", "")
	t.Setenv("MOLE_CONFIG", "relative.json")
	if _, err := NewSpec(t.TempDir(), "relative.json"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative MOLE_CONFIG error = %v", err)
	}
}

func TestExecutableRejectsGoBuildCache(t *testing.T) {
	if err := validateExecutable("/tmp/go-build123/b001/exe/mole"); err == nil {
		t.Fatal("go-build executable should be rejected")
	}
}

func TestServiceConfigTemplates(t *testing.T) {
	home := t.TempDir()
	exe := testExecutable(t)
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: exe, GOOS: "darwin"}
	cfg, err := s.ServiceConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "io.mole.agent."+shortHash(home) && cfg.Name != "io.mole.agent" {
		t.Fatalf("Name = %q", cfg.Name)
	}
	if cfg.Executable != exe || len(cfg.Arguments) != 1 || cfg.Arguments[0] != "serve" {
		t.Fatalf("exec/args = %q %v", cfg.Executable, cfg.Arguments)
	}
	if cfg.EnvVars["MOLE_HOME"] != home {
		t.Fatalf("MOLE_HOME env = %q", cfg.EnvVars["MOLE_HOME"])
	}
	if cfg.Option["LaunchdConfig"] == nil || cfg.Option["UserService"] != true {
		t.Fatalf("launchd options = %#v", cfg.Option)
	}
}

func TestServiceConfigRejectsRelativeExecutable(t *testing.T) {
	s := Spec{HomePath: t.TempDir(), Executable: "mole", GOOS: "darwin"}
	if _, err := s.ServiceConfig(""); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative executable error = %v", err)
	}
}

// Foreground `mole serve` must work under `go run`, so building a service
// config must not reject a temporary go-build executable. Only Install does.
func TestServiceConfigAllowsGoBuildExecutable(t *testing.T) {
	home := t.TempDir()
	exe := "/tmp/go-build123/b001/exe/mole"
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: exe, GOOS: "darwin"}
	cfg, err := s.ServiceConfig("")
	if err != nil {
		t.Fatalf("ServiceConfig() error = %v", err)
	}
	if cfg.Executable != exe {
		t.Fatalf("Executable = %q, want %q", cfg.Executable, exe)
	}
}

func TestServiceConfigOnlyBootstrapEnv(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "custom", "config.json")
	t.Setenv("MOLE_CONFIG", configPath)
	s := Spec{HomePath: home, ConfigPath: configPath, Executable: testExecutable(t), GOOS: "linux"}
	cfg, err := s.ServiceConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkingDirectory != home {
		t.Fatalf("WorkingDirectory = %q", cfg.WorkingDirectory)
	}
	if len(cfg.EnvVars) != 2 {
		t.Fatalf("EnvVars = %#v, want only MOLE_HOME and MOLE_CONFIG", cfg.EnvVars)
	}
	if cfg.EnvVars["MOLE_HOME"] != home || cfg.EnvVars["MOLE_CONFIG"] != configPath {
		t.Fatalf("EnvVars = %#v", cfg.EnvVars)
	}
	if cfg.Option["UserService"] != true || cfg.Option["SystemdScript"] != systemdConfig {
		t.Fatalf("systemd options = %#v", cfg.Option)
	}
	if cfg.Name != "mole-"+shortHash(home) {
		t.Fatalf("Name = %q", cfg.Name)
	}
}

func TestServiceConfigOmitsConfigEnvWhenUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MOLE_CONFIG", "")
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: testExecutable(t), GOOS: "linux"}
	cfg, err := s.ServiceConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.EnvVars["MOLE_CONFIG"]; ok {
		t.Fatalf("MOLE_CONFIG should be absent: %#v", cfg.EnvVars)
	}
}

func TestServiceConfigWindowsOptions(t *testing.T) {
	home := t.TempDir()
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: testExecutable(t), GOOS: "windows"}
	cfg, err := s.ServiceConfig("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "Mole-"+shortHash(home) {
		t.Fatalf("Name = %q", cfg.Name)
	}
	wantUser, err := currentUsername()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserName != wantUser {
		t.Fatalf("UserName = %q, want %q", cfg.UserName, wantUser)
	}
	if cfg.Option["StartType"] != "automatic" {
		t.Fatalf("StartType = %#v", cfg.Option["StartType"])
	}
	if cfg.Option["OnFailure"] != "restart" {
		t.Fatalf("OnFailure = %#v", cfg.Option["OnFailure"])
	}
	if cfg.Option["OnFailureDelayDuration"] != "10s" {
		t.Fatalf("OnFailureDelayDuration = %#v", cfg.Option["OnFailureDelayDuration"])
	}
	if cfg.Option["Password"] != "s3cret" {
		t.Fatalf("Password = %#v", cfg.Option["Password"])
	}

	cfg, err = s.ServiceConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Option["Password"]; ok {
		t.Fatalf("empty password should not be stored: %#v", cfg.Option)
	}
}

func TestServiceConfigDarwinDoesNotCreateLogDirectory(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	s := Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: testExecutable(t), GOOS: "darwin"}
	cfg, err := s.ServiceConfig("")
	if err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(home, "logs")
	if _, err := os.Stat(logDir); !os.IsNotExist(err) {
		t.Fatalf("ServiceConfig must not create %q, stat error = %v", logDir, err)
	}
	if cfg.Option["LogDirectory"] != logDir {
		t.Fatalf("LogDirectory = %#v, want %q", cfg.Option["LogDirectory"], logDir)
	}
}

func TestEnsureLogDirDarwin(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	s := Spec{HomePath: home, GOOS: "darwin"}
	if err := ensureLogDir(s); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(home, "logs")
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("log directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", logDir)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("log directory mode = %o, want 700", perm)
	}

	other := Spec{HomePath: filepath.Join(t.TempDir(), "other"), GOOS: "linux"}
	if err := ensureLogDir(other); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other.HomePath, "logs")); !os.IsNotExist(err) {
		t.Fatalf("non-darwin ensureLogDir created a directory, stat error = %v", err)
	}
}

func TestUnitPath(t *testing.T) {
	userHome, _ := os.UserHomeDir()
	s := Spec{HomePath: filepath.Join(t.TempDir(), "mole-dev"), GOOS: "darwin"}
	got, err := s.UnitPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(userHome, "Library", "LaunchAgents", s.Name()+".plist")
	if got != want {
		t.Fatalf("darwin UnitPath = %q, want %q", got, want)
	}

	s.GOOS = "linux"
	got, err = s.UnitPath()
	if err != nil {
		t.Fatal(err)
	}
	want = filepath.Join(userHome, ".config", "systemd", "user", s.Name()+".service")
	if got != want {
		t.Fatalf("linux UnitPath = %q, want %q", got, want)
	}
}

func TestEnvPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "mole-dev")
	s := Spec{HomePath: base, ConfigPath: filepath.Join(base, "conf", "config.json")}
	if got, want := s.EnvPath(), filepath.Join(base, "conf", ".env"); got != want {
		t.Fatalf("EnvPath = %q, want %q", got, want)
	}
}

// testExecutable returns an absolute path outside the go-build tree that is
// valid on any host OS.
func testExecutable(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mole")
}
