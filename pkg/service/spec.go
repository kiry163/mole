// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

// Package service defines the agent's OS service identity and the per-platform
// service manager configuration used to install it.
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	svc "github.com/kardianos/service"

	"github.com/kiry163/mole/pkg/config"
)

// kardianos/service option keys. The library only exports the Windows keys on
// Windows builds, but the platform switch below is keyed on Spec.GOOS so the
// whole file must compile everywhere. The key strings are part of its public
// configuration contract.
const (
	optUserService            = "UserService"
	optRunAtLoad              = "RunAtLoad"
	optKeepAlive              = "KeepAlive"
	optLogDirectory           = "LogDirectory"
	optLaunchdConfig          = "LaunchdConfig"
	optSystemdScript          = "SystemdScript"
	optStartType              = "StartType"
	optStartTypeAutomatic     = "automatic"
	optOnFailure              = "OnFailure"
	optOnFailureRestart       = "restart"
	optOnFailureDelayDuration = "OnFailureDelayDuration"
	optPassword               = "Password"
)

// Spec identifies one mole instance and how it should be registered as a
// user service. HomePath and ConfigPath are absolute paths.
type Spec struct {
	HomePath   string
	ConfigPath string
	Executable string
	GOOS       string // empty -> runtime.GOOS; tests set it
}

// NewSpec resolves the service identity for a mole home. Explicit bootstrap
// environment overrides must be absolute: the service manager starts the
// process from an unrelated working directory, so relative paths cannot be
// interpreted later.
func NewSpec(home, configPath string) (Spec, error) {
	if v := os.Getenv(config.EnvHome); v != "" && !filepath.IsAbs(v) {
		return Spec{}, fmt.Errorf("MOLE_HOME must be an absolute path: %q", v)
	}
	if v := os.Getenv(config.EnvConfig); v != "" && !filepath.IsAbs(v) {
		return Spec{}, fmt.Errorf("MOLE_CONFIG must be an absolute path: %q", v)
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve MOLE_HOME: %w", err)
	}
	if configPath == "" {
		configPath = filepath.Join(absHome, "config.json")
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve MOLE_CONFIG: %w", err)
	}
	exe, err := resolveExecutable()
	if err != nil {
		return Spec{}, err
	}
	return Spec{HomePath: filepath.Clean(absHome), ConfigPath: absConfig, Executable: exe}, nil
}

func (s Spec) goos() string {
	if s.GOOS != "" {
		return s.GOOS
	}
	return runtime.GOOS
}

// Name returns the service identifier. Custom homes get a stable short hash so
// multiple instances can be installed side by side.
func (s Spec) Name() string {
	h := shortHash(s.HomePath)
	if isDefaultHome(s.HomePath) {
		switch s.goos() {
		case "darwin":
			return "io.mole.agent"
		case "windows":
			return "Mole-" + h
		default:
			return "mole"
		}
	}
	switch s.goos() {
	case "darwin":
		return "io.mole.agent." + h
	case "windows":
		return "Mole-" + h
	default:
		return "mole-" + h
	}
}

// DisplayName is the human readable label shown by service managers.
func (s Spec) DisplayName() string {
	if isDefaultHome(s.HomePath) {
		return "Mole Agent"
	}
	return "Mole Agent (" + s.HomePath + ")"
}

// EnvPath returns the instance .env path next to the config file.
func (s Spec) EnvPath() string { return config.EnvFilePath(s.ConfigPath) }

// UnitPath returns where the platform service definition is installed. Windows
// keeps definitions in the service database, so it has no file path.
func (s Spec) UnitPath() (string, error) {
	switch s.goos() {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "LaunchAgents", s.Name()+".plist"), nil
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "systemd", "user", s.Name()+".service"), nil
	default:
		return "", nil
	}
}

// ServiceConfig builds the kardianos/service configuration. It carries only
// bootstrap location (MOLE_HOME, MOLE_CONFIG); secrets stay in .env and are
// loaded by the program before the gateway starts.
func (s Spec) ServiceConfig(password string) (*svc.Config, error) {
	if s.Executable == "" || !filepath.IsAbs(s.Executable) {
		return nil, fmt.Errorf("service executable must be absolute: %q", s.Executable)
	}
	cfg := &svc.Config{
		Name:             s.Name(),
		DisplayName:      s.DisplayName(),
		Description:      "Mole personal AI assistant",
		Executable:       s.Executable,
		Arguments:        []string{"serve"},
		WorkingDirectory: s.HomePath,
		EnvVars:          map[string]string{"MOLE_HOME": s.HomePath},
		Option:           svc.KeyValue{},
	}
	if v := os.Getenv(config.EnvConfig); v != "" {
		cfg.EnvVars["MOLE_CONFIG"] = v
	}
	switch s.goos() {
	case "darwin":
		cfg.Option[optUserService] = true
		cfg.Option[optRunAtLoad] = true
		cfg.Option[optKeepAlive] = true
		cfg.Option[optLogDirectory] = filepath.Join(s.HomePath, "logs")
		cfg.Option[optLaunchdConfig] = launchdConfig
	case "linux":
		cfg.Option[optUserService] = true
		cfg.Option[optSystemdScript] = systemdConfig
	case "windows":
		username, err := currentUsername()
		if err != nil {
			return nil, err
		}
		cfg.UserName = username
		cfg.Option[optStartType] = optStartTypeAutomatic
		cfg.Option[optOnFailure] = optOnFailureRestart
		cfg.Option[optOnFailureDelayDuration] = "10s"
		if password != "" {
			cfg.Option[optPassword] = password
		}
	}
	return cfg, nil
}

// ensureLogDir creates launchd's log directory before install. launchd opens
// StandardOutPath/StandardErrorPath before exec and does not create
// intermediate directories, so a fresh home would fail to start. Read-only
// lifecycle commands must not create it.
func ensureLogDir(s Spec) error {
	if s.goos() != "darwin" {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(s.HomePath, "logs"), 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	return nil
}

// shortHash returns the first 8 hex characters of SHA-256 over the cleaned
// home path. It names service instances without leaking the path.
func shortHash(home string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(home)))
	return hex.EncodeToString(sum[:4])
}

func isDefaultHome(home string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(home) == filepath.Join(userHome, ".mole")
}

func currentUsername() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if u.Username == "" {
		return "", fmt.Errorf("resolve current user: empty username")
	}
	return u.Username, nil
}

// resolveExecutable resolves the running binary to an absolute path suitable
// for a service definition.
func resolveExecutable() (string, error) {
	exe := os.Args[0]
	if !strings.ContainsRune(exe, os.PathSeparator) {
		found, err := exec.LookPath(exe)
		if err != nil {
			return "", fmt.Errorf("resolve executable: %w", err)
		}
		exe = found
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// validateExecutable rejects paths that vanish after the install command
// returns, such as `go run` build directories.
func validateExecutable(path string) error {
	if strings.Contains(filepath.ToSlash(path), "/go-build") {
		return fmt.Errorf("refusing to install a go-build temporary executable: %s", path)
	}
	return nil
}
