// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	svc "github.com/kardianos/service"
)

// noopProgram satisfies svc.Interface for manager-only operations. Install,
// start, and stop are handled by the platform service manager; the process is
// never hosted in this command.
type noopProgram struct{}

func (noopProgram) Start(svc.Service) error { return nil }

func (noopProgram) Stop(svc.Service) error { return nil }

// Manager performs service lifecycle operations for one Spec.
type Manager struct {
	Spec    Spec
	Service svc.Service
}

// NewManager resolves the service configuration and platform implementation.
// ServiceConfig errors (missing or invalid executable) surface here.
func NewManager(spec Spec, password string) (*Manager, error) {
	cfg, err := spec.ServiceConfig(password)
	if err != nil {
		return nil, err
	}
	s, err := svc.New(noopProgram{}, cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{Spec: spec, Service: s}, nil
}

// ValidateInstall rejects executables that vanish after the install command
// returns, such as `go run` build directories. It is separate from Install so
// destructive commands (reinstall) can fail before stopping or removing a unit.
func (m *Manager) ValidateInstall() error {
	return validateExecutable(m.Spec.Executable)
}

// Install validates the executable, then registers the unit. Foreground runs
// do not install a unit and may use a temporary go-build executable.
func (m *Manager) Install() error {
	if err := m.ValidateInstall(); err != nil {
		return err
	}
	if err := ensureLogDir(m.Spec); err != nil {
		return err
	}
	return m.Service.Install()
}

func (m *Manager) Uninstall() error { return m.Service.Uninstall() }

func (m *Manager) Start() error { return m.Service.Start() }

func (m *Manager) Stop() error { return m.Service.Stop() }

// Restart restarts a running service and starts a stopped one. kardianos
// Restart stops first, which fails on launchd and the Windows SCM when the
// service is installed but not running.
func (m *Manager) Restart() error {
	st, err := m.Service.Status()
	if err != nil {
		return err
	}
	if st != svc.StatusRunning {
		return m.Service.Start()
	}
	return m.Service.Restart()
}

func (m *Manager) Status() (svc.Status, error) { return m.Service.Status() }
