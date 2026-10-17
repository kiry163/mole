// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	svc "github.com/kardianos/service"
)

// fakeManagerService records lifecycle calls and returns canned status.
// Embedding the interface keeps unused methods nil.
type fakeManagerService struct {
	svc.Service
	status  svc.Status
	err     error
	start   error
	restart error
	install error
	calls   []string
}

func (f *fakeManagerService) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeManagerService) Status() (svc.Status, error) {
	f.record("status")
	return f.status, f.err
}

func (f *fakeManagerService) Start() error {
	f.record("start")
	return f.start
}

func (f *fakeManagerService) Restart() error {
	f.record("restart")
	return f.restart
}

func (f *fakeManagerService) Install() error {
	f.record("install")
	return f.install
}

func TestManagerRestartStartsStoppedService(t *testing.T) {
	fake := &fakeManagerService{status: svc.StatusStopped}
	m := &Manager{Service: fake}
	if err := m.Restart(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fake.calls, ","); got != "status,start" {
		t.Fatalf("calls = %q, want %q", got, "status,start")
	}
}

func TestManagerRestartRestartsRunningService(t *testing.T) {
	fake := &fakeManagerService{status: svc.StatusRunning}
	m := &Manager{Service: fake}
	if err := m.Restart(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fake.calls, ","); got != "status,restart" {
		t.Fatalf("calls = %q, want %q", got, "status,restart")
	}
}

func TestManagerRestartStatusError(t *testing.T) {
	wantErr := errors.New("status unavailable")
	fake := &fakeManagerService{err: wantErr}
	m := &Manager{Service: fake}
	if err := m.Restart(); !errors.Is(err, wantErr) {
		t.Fatalf("Restart() error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(fake.calls, ","); got != "status" {
		t.Fatalf("calls = %q, want status only", got)
	}
}

func TestManagerInstallCreatesDarwinLogDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	spec := Spec{
		HomePath:   home,
		ConfigPath: filepath.Join(home, "config.json"),
		Executable: testExecutable(t),
		GOOS:       "darwin",
	}
	fake := &fakeManagerService{}
	m := &Manager{Spec: spec, Service: fake}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(home, "logs")
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("Install did not create log directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("log directory mode = %o, want 700", perm)
	}
	if got := strings.Join(fake.calls, ","); got != "install" {
		t.Fatalf("calls = %q, want install", got)
	}
}
