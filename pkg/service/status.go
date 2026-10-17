// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	svc "github.com/kardianos/service"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/pid"
)

const (
	defaultGatewayHost = "127.0.0.1"
	defaultGatewayPort = 18790

	healthProbeTimeout = time.Second
	readyPollInterval  = 200 * time.Millisecond
)

// probeHTTPClient is shared by health and readiness checks. It deliberately
// disables environment proxies: a local gateway must be probed directly.
var probeHTTPClient = &http.Client{
	Timeout:   healthProbeTimeout,
	Transport: &http.Transport{Proxy: nil},
}

// Health values reported by Status.Health.
const (
	HealthReady       = "ready"
	HealthDegraded    = "degraded"
	HealthUnreachable = "unreachable"
)

// Status is the observable state of one mole instance. It intentionally has no
// token field: service status output must never expose credentials.
type Status struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Home          string `json:"home"`
	ConfigPath    string `json:"config_path"`
	Executable    string `json:"executable"`
	UnitPath      string `json:"unit_path,omitempty"`
	Installed     bool   `json:"installed"`
	Running       bool   `json:"running"`
	ManualRunning bool   `json:"manual_running"`
	PID           int    `json:"pid,omitempty"`
	URL           string `json:"url,omitempty"`
	Health        string `json:"health,omitempty"`
	EnvChanged    bool   `json:"env_changed"`
	ConfigError   string `json:"config_error,omitempty"`
}

// URL builds the browser URL for a gateway listen address. Wildcard binds are
// shown as loopback, IPv6 literals are bracketed, and uninitialized instances
// append /setup.
func URL(host string, port int, setup bool) string {
	h := strings.TrimSpace(host)
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		h = defaultGatewayHost
	}
	if port <= 0 {
		port = defaultGatewayPort
	}
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	u := "http://" + h + ":" + strconv.Itoa(port)
	if setup {
		u += "/setup"
	}
	return u
}

// CollectStatus reports the service, process, config, and health state. A
// service manager is optional: without one, a live PID file means the gateway
// was started manually. Config load failures are recorded in ConfigError so the
// rest of the status is still useful.
func (m *Manager) CollectStatus() (Status, error) {
	st := Status{
		Name:        m.Spec.Name(),
		DisplayName: m.Spec.DisplayName(),
		Home:        m.Spec.HomePath,
		ConfigPath:  m.Spec.ConfigPath,
		Executable:  m.Spec.Executable,
	}
	if unit, err := m.Spec.UnitPath(); err == nil {
		st.UnitPath = unit
	}

	var statusErr error
	if m.Service != nil {
		serviceStatus, err := m.Service.Status()
		switch {
		case errors.Is(err, svc.ErrNotInstalled):
			// First run: not installed is a valid status, not a check failure.
		case err != nil:
			statusErr = fmt.Errorf("service status: %w", err)
		default:
			st.Installed = serviceStatus != svc.StatusUnknown
			st.Running = serviceStatus == svc.StatusRunning
		}
	}

	pidData := pid.ReadPidFileWithCheck(m.Spec.HomePath)
	if pidData != nil {
		st.PID = pidData.PID
	}

	cfg, cfgErr := config.LoadConfig(m.Spec.ConfigPath)
	if cfgErr != nil {
		st.ConfigError = cfgErr.Error()
	}

	host, port := defaultGatewayHost, defaultGatewayPort
	if cfg != nil {
		host, port = cfg.Gateway.Host, cfg.Gateway.Port
	}
	if pidData != nil {
		if pidData.Host != "" {
			host = pidData.Host
		}
		if pidData.Port != 0 {
			port = pidData.Port
		}
	}

	setup := cfg == nil || strings.TrimSpace(cfg.Dashboard.Token.String()) == ""
	baseURL := URL(host, port, false)
	st.URL = URL(host, port, setup)

	st.ManualRunning = pidData != nil && !st.Running
	if st.ManualRunning {
		st.Running = true
	}

	if pidData != nil {
		st.EnvChanged = envChanged(m.Spec)
	}

	if st.Running {
		st.Health = probeHealth(baseURL)
	}

	return st, statusErr
}

// WaitReady polls the gateway /ready endpoint until it reports success or the
// timeout expires.
func WaitReady(spec Spec, timeout time.Duration) error {
	cfg, err := config.LoadConfig(spec.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config for ready check: %w", err)
	}
	host, port := defaultGatewayHost, defaultGatewayPort
	if cfg != nil {
		host, port = cfg.Gateway.Host, cfg.Gateway.Port
	}
	base := URL(host, port, false)

	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		ok, err := endpointOK(probeHTTPClient, base+"/ready")
		if ok {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("/ready returned a non-ready status")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if remaining < readyPollInterval {
			time.Sleep(remaining)
		} else {
			time.Sleep(readyPollInterval)
		}
	}
	return fmt.Errorf("gateway did not become ready at %s within %s: %w", base, timeout, lastErr)
}

// envChanged reports whether the instance .env is newer than the PID file,
// which means the running process still has the old environment.
func envChanged(spec Spec) bool {
	envInfo, err := os.Stat(spec.EnvPath())
	if err != nil {
		return false
	}
	pidInfo, err := os.Stat(pid.FilePath(spec.HomePath))
	if err != nil {
		return false
	}
	return envInfo.ModTime().After(pidInfo.ModTime())
}

// probeHealth reports ready when /ready succeeds, degraded when only /health
// responds, and unreachable otherwise.
func probeHealth(url string) string {
	if ok, _ := endpointOK(probeHTTPClient, url+"/ready"); ok {
		return HealthReady
	}
	if ok, _ := endpointOK(probeHTTPClient, url+"/health"); ok {
		return HealthDegraded
	}
	return HealthUnreachable
}

func endpointOK(client *http.Client, url string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	return resp.StatusCode == http.StatusOK, nil
}
