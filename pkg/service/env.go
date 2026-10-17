// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/envfile"
	"github.com/kiry163/mole/pkg/logger"
)

// RefreshKeys are the shell variables imported by `mole service env refresh`.
// The list is intentionally small: paths, locale/timezone, and proxy/TLS
// settings survive a service manager's minimal environment, while secrets and
// arbitrary variables stay out of the instance .env file.
var RefreshKeys = []string{
	"PATH",
	"TZ",
	"LANG",
	"LC_ALL",
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"ALL_PROXY",
	"SSL_CERT_FILE",
	"SSL_CERT_DIR",
}

// LoadEnv applies the instance .env file to the process environment before the
// gateway starts. Bootstrap variables in the file are ignored with a warning:
// the service definition is the only source of MOLE_HOME and MOLE_CONFIG.
func LoadEnv(configPath string) error {
	path := config.EnvFilePath(configPath)
	ignored, err := envfile.LoadAndApply(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	if len(ignored) > 0 {
		logger.Warnf("ignoring bootstrap variables in %s: %s", path, strings.Join(ignored, ", "))
	}
	return nil
}

// RefreshEnv copies RefreshKeys from the current process environment into the
// instance .env file, preserving comments and unrelated lines. With dryRun it
// reports the updates without touching the file.
func RefreshEnv(configPath string, dryRun bool) (map[string]string, error) {
	updates := make(map[string]string, len(RefreshKeys))
	for _, key := range RefreshKeys {
		if value, ok := os.LookupEnv(key); ok {
			updates[key] = value
		}
	}
	if dryRun || len(updates) == 0 {
		return updates, nil
	}
	path := config.EnvFilePath(configPath)
	if err := envfile.Update(path, updates); err != nil {
		return nil, fmt.Errorf("update %s: %w", path, err)
	}
	return updates, nil
}

// EnsureEnv creates the instance .env file from the current shell when it does
// not exist yet. An existing file is never overwritten.
func EnsureEnv(configPath string) error {
	path := config.EnvFilePath(configPath)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	_, err := RefreshEnv(configPath, false)
	return err
}
