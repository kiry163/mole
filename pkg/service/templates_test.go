// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"strings"
	"testing"
)

func TestTemplatesContainRequiredDirectives(t *testing.T) {
	for _, want := range []string{"RunAtLoad", "KeepAlive", "ThrottleInterval", "ExitTimeOut", "ProcessType"} {
		if !strings.Contains(launchdConfig, want) {
			t.Errorf("launchd template missing %s", want)
		}
	}
	for _, want := range []string{"WantedBy=default.target", "Restart=on-failure", "RestartSec=10", "TimeoutStopSec=45"} {
		if !strings.Contains(systemdConfig, want) {
			t.Errorf("systemd template missing %s", want)
		}
	}
}
