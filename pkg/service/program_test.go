// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	svc "github.com/kardianos/service"

	"github.com/kiry163/mole/pkg/gateway"
)

func newTestProgram(t *testing.T) *Program {
	t.Helper()
	p := NewProgram(gateway.Options{ConfigPath: filepath.Join(t.TempDir(), "config.json")})
	p.startTimeout = time.Second
	p.stopTimeout = time.Second
	return p
}

// fakeService embeds the interface so tests only need to implement the calls
// they exercise.
type fakeService struct {
	svc.Service

	mu       sync.Mutex
	calls    []string
	status   svc.Status
	statusOK error
	stopErr  error
	stopped  chan struct{}
	stopOnce sync.Once
}

func (f *fakeService) record(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
}

func (f *fakeService) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeService) Start() error { f.record("start"); return nil }

func (f *fakeService) Stop() error {
	f.record("stop")
	if f.stopped != nil {
		f.stopOnce.Do(func() { close(f.stopped) })
	}
	return f.stopErr
}

func (f *fakeService) Restart() error { f.record("restart"); return nil }

func (f *fakeService) Install() error { f.record("install"); return nil }

func (f *fakeService) Uninstall() error { f.record("uninstall"); return nil }

func (f *fakeService) Status() (svc.Status, error) {
	f.record("status")
	return f.status, f.statusOK
}

func TestProgramStartReadyAndStop(t *testing.T) {
	p := newTestProgram(t)
	runStarted := make(chan struct{})
	runDone := make(chan struct{})
	p.run = func(ctx context.Context, opts gateway.Options) error {
		close(runStarted)
		opts.NotifyStarted(nil)
		<-ctx.Done()
		close(runDone)
		return nil
	}

	if err := p.Start(nil); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-runStarted:
	case <-time.After(time.Second):
		t.Fatal("gateway run did not start")
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("gateway run did not stop")
	}
	// A second stop must not block or repeat the gateway shutdown.
	if err := p.Stop(nil); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestProgramStartGatewayError(t *testing.T) {
	p := newTestProgram(t)
	p.run = func(_ context.Context, opts gateway.Options) error {
		err := errors.New("boom")
		opts.NotifyStarted(err)
		return err
	}

	if err := p.Start(nil); err == nil || err.Error() != "boom" {
		t.Fatalf("Start() error = %v, want boom", err)
	}
}

func TestProgramStartTimeout(t *testing.T) {
	p := newTestProgram(t)
	p.startTimeout = 20 * time.Millisecond
	p.run = func(ctx context.Context, _ gateway.Options) error {
		<-ctx.Done()
		return ctx.Err()
	}

	err := p.Start(nil)
	if err == nil || !strings.Contains(err.Error(), "ready") {
		t.Fatalf("Start() error = %v, want readiness timeout", err)
	}
}

func TestProgramStopTimeout(t *testing.T) {
	p := newTestProgram(t)
	release := make(chan struct{})
	p.stopTimeout = 20 * time.Millisecond
	p.run = func(_ context.Context, opts gateway.Options) error {
		opts.NotifyStarted(nil)
		<-release
		return nil
	}

	if err := p.Start(nil); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	err := p.Stop(nil)
	if err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("Stop() error = %v, want stop timeout", err)
	}
	close(release)
}

func TestProgramUnexpectedExitStopsService(t *testing.T) {
	p := newTestProgram(t)
	p.interactive = func() bool { return false }
	p.run = func(_ context.Context, opts gateway.Options) error {
		opts.NotifyStarted(nil)
		return errors.New("gateway crashed")
	}
	s := &fakeService{stopped: make(chan struct{})}

	if err := p.Start(s); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-s.stopped:
	case <-time.After(time.Second):
		t.Fatal("service Stop was not called after gateway exit")
	}
}

func TestUnexpectedExitAction(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		hasService  bool
		want        programExitAction
	}{
		{"no service", false, false, exitActionNone},
		{"interactive foreground exits", true, true, exitActionExitProcess},
		{"managed service stops unit", false, true, exitActionStopService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unexpectedExitAction(tt.interactive, tt.hasService); got != tt.want {
				t.Fatalf("unexpectedExitAction(%v, %v) = %v, want %v",
					tt.interactive, tt.hasService, got, tt.want)
			}
		})
	}
}

func TestProgramStartLoadsEnvAndTimezone(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	env := "# bootstrap\nMOLE_PROGRAM_TEST_VALUE=from-env\nTZ=UTC\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOLE_PROGRAM_TEST_VALUE", "before")
	t.Setenv("TZ", "")
	oldLocal := time.Local
	t.Cleanup(func() { time.Local = oldLocal })

	p := NewProgram(gateway.Options{ConfigPath: configPath})
	p.startTimeout = time.Second
	p.stopTimeout = time.Second
	observed := make(chan string, 1)
	p.run = func(ctx context.Context, opts gateway.Options) error {
		observed <- os.Getenv("MOLE_PROGRAM_TEST_VALUE")
		opts.NotifyStarted(nil)
		<-ctx.Done()
		return nil
	}

	if err := p.Start(nil); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := <-observed; got != "from-env" {
		t.Fatalf("gateway saw env value %q, want from-env", got)
	}
	if got := time.Local.String(); got != "UTC" {
		t.Fatalf("time.Local = %q, want UTC", got)
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestRunPropagatesSpecError(t *testing.T) {
	err := Run(Spec{Executable: "mole"}, NewProgram(gateway.Options{}))
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("Run() error = %v, want absolute executable error", err)
	}
}

func TestNewManagerPropagatesServiceConfigError(t *testing.T) {
	spec := Spec{HomePath: t.TempDir(), Executable: "mole", GOOS: "darwin"}
	_, err := NewManager(spec, "")
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("NewManager() error = %v, want absolute executable error", err)
	}
}

func TestManagerInstallRejectsGoBuildExecutable(t *testing.T) {
	s := &fakeService{}
	m := &Manager{
		Spec:    Spec{Executable: "/tmp/go-build123/b001/exe/mole"},
		Service: s,
	}
	err := m.Install()
	if err == nil || !strings.Contains(err.Error(), "go-build") {
		t.Fatalf("Install() error = %v, want go-build rejection", err)
	}
	if calls := s.recorded(); len(calls) != 0 {
		t.Fatalf("service received calls %v; Install must fail before registering", calls)
	}
}

func TestManagerDelegatesToService(t *testing.T) {
	s := &fakeService{status: svc.StatusRunning}
	m := &Manager{Spec: Spec{HomePath: t.TempDir(), Executable: testExecutable(t), GOOS: "linux"}, Service: s}

	for name, call := range map[string]func() error{
		"install":   m.Install,
		"uninstall": m.Uninstall,
		"start":     m.Start,
		"stop":      m.Stop,
		"restart":   m.Restart,
	} {
		if err := call(); err != nil {
			t.Fatalf("%s() error = %v", name, err)
		}
	}
	status, err := m.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != svc.StatusRunning {
		t.Fatalf("Status() = %v, want running", status)
	}

	got := map[string]bool{}
	for _, call := range s.recorded() {
		got[call] = true
	}
	for _, want := range []string{"install", "uninstall", "start", "stop", "restart", "status"} {
		if !got[want] {
			t.Errorf("service did not receive %s; calls = %v", want, s.recorded())
		}
	}
}
