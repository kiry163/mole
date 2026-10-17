package service

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	managersvc "github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kiry163/mole/pkg/pid"
	svc "github.com/kiry163/mole/pkg/service"
)

// fakeService implements just enough of managersvc.Service for status
// collection and guarded start tests; embedding the interface keeps the unused
// methods nil.
type fakeService struct {
	managersvc.Service
	status       managersvc.Status
	statusErr    error
	startErr     error
	installCalls int
	startCalls   int
	calls        []string
}

func (f *fakeService) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeService) Status() (managersvc.Status, error) {
	f.record("status")
	return f.status, f.statusErr
}

func (f *fakeService) Install() error {
	f.record("install")
	f.installCalls++
	return nil
}

func (f *fakeService) Start() error {
	f.record("start")
	f.startCalls++
	return f.startErr
}

func (f *fakeService) Stop() error {
	f.record("stop")
	return nil
}

func (f *fakeService) Uninstall() error {
	f.record("uninstall")
	return nil
}

func (f *fakeService) Restart() error {
	f.record("restart")
	return nil
}

func findSub(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("subcommand %q not found", name)
	return nil
}

func TestNewServiceCommand(t *testing.T) {
	cmd := NewServiceCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "service", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.True(t, cmd.HasSubCommands())

	names := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	sort.Strings(names)
	assert.Equal(t, []string{
		"env", "install", "logs", "reinstall", "restart",
		"start", "status", "stop", "uninstall",
	}, names)
}

func TestServiceFlags(t *testing.T) {
	cmd := NewServiceCommand()

	install := findSub(t, cmd, "install")
	assert.NotNil(t, install.Flags().Lookup("now"))
	assert.Nil(t, install.Flags().Lookup("system"))

	reinstall := findSub(t, cmd, "reinstall")
	assert.NotNil(t, reinstall.Flags().Lookup("now"))

	status := findSub(t, cmd, "status")
	assert.NotNil(t, status.Flags().Lookup("json"))

	logs := findSub(t, cmd, "logs")
	follow := logs.Flags().Lookup("follow")
	require.NotNil(t, follow)
	assert.Equal(t, "f", follow.Shorthand)
	lines := logs.Flags().Lookup("lines")
	require.NotNil(t, lines)
	assert.Equal(t, "200", lines.DefValue)

	env := findSub(t, cmd, "env")
	refresh := findSub(t, env, "refresh")
	assert.NotNil(t, refresh.Flags().Lookup("restart"))
	assert.NotNil(t, refresh.Flags().Lookup("dry-run"))
}

func TestServiceNoSubcommandShowsHelp(t *testing.T) {
	cmd := NewServiceCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "Usage:")
	assert.Contains(t, out.String(), "install")
	assert.Contains(t, out.String(), "status")
}

func TestEnvNoSubcommandShowsHelp(t *testing.T) {
	cmd := NewServiceCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"env"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "refresh")
}

// stubSpec points command resolution at a temp instance and records whether the
// service manager was constructed.
func stubSpec(t *testing.T, spec svc.Spec) *bool {
	t.Helper()
	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	called := false
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		called = true
		return &svc.Manager{Spec: s, Service: &fakeService{status: managersvc.StatusStopped}}, nil
	}
	return &called
}

func TestEnvRefreshDryRun(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	managerCalled := stubSpec(t, svc.Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/mole"})
	t.Setenv("TZ", "UTC")

	cmd := NewServiceCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"env", "refresh", "--dry-run"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "TZ=UTC")
	assert.Contains(t, out.String(), filepath.Join(home, ".env"))
	_, err := os.Stat(filepath.Join(home, ".env"))
	assert.True(t, os.IsNotExist(err), "dry-run must not create .env")
	assert.False(t, *managerCalled, "plain env refresh must not touch the service manager")
}

func TestEnvRefreshRejectsDryRunRestart(t *testing.T) {
	home := t.TempDir()
	stubSpec(t, svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"})

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"env", "refresh", "--dry-run", "--restart"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be combined")
}

func TestEnvRefreshRestartRequiresInstalled(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: &fakeService{statusErr: managersvc.ErrNotInstalled}}, nil
	}

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"env", "refresh", "--restart"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestEnvRefreshRestartStartsStoppedService(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	startErr := errors.New("start failed")
	fake := &fakeService{status: managersvc.StatusStopped, startErr: startErr}
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: fake}, nil
	}

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"env", "refresh", "--restart"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, startErr)
	assert.Contains(t, fake.calls, "start")
	assert.NotContains(t, fake.calls, "restart")
}

func TestEnvRefreshRestartRestartsRunningService(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	fake := &fakeService{status: managersvc.StatusRunning}
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: fake}, nil
	}

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"env", "refresh", "--restart"})
	// Readiness fails because the test instance has no config, but the
	// lifecycle call already happened.
	_ = cmd.Execute()
	assert.Contains(t, fake.calls, "restart")
	assert.NotContains(t, fake.calls, "start")
}

func TestStartRefusesManualInstance(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	stubSpec(t, spec)
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pid.RemovePidFile(home) })

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"start"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop that instance first")
	assert.Contains(t, err.Error(), strconv.Itoa(os.Getpid()))
}

func TestStopLeavesManualInstance(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	stubSpec(t, spec)
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pid.RemovePidFile(home) })

	cmd := NewServiceCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"stop"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "running manually")
	assert.NotNil(t, pid.ReadPidFileWithCheck(home), "manual instance must not be stopped")
}

func TestServiceCommandsRejectArgs(t *testing.T) {
	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"status", "extra"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "accepts 0 arg"))
}

// waitUntilReady must apply the instance .env before reading config: gateway
// readiness checks load config, and model settings may use $VAR references.
func TestWaitUntilReadyLoadsEnv(t *testing.T) {
	const envKey = "MOLE_TASK4_API_BASE"
	t.Setenv(envKey, "")
	require.NoError(t, os.Unsetenv(envKey))

	var readyHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&readyHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, portText, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	home := t.TempDir()
	configPath := filepath.Join(home, "config.json")
	configJSON := fmt.Sprintf(`{
	  "version": 4,
	  "gateway": {"host": %q, "port": %d},
	  "model_list": [{
	    "model_name": "m",
	    "provider": "openai",
	    "model": "gpt-test",
	    "api_base": "$%s",
	    "api_keys": ["sk-test"]
	  }],
	  "agents": {"defaults": {"model_name": "m"}}
	}`, host, port, envKey)
	require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".env"), []byte(envKey+"=https://example.test/v1\n"), 0o600))

	spec := svc.Spec{HomePath: home, ConfigPath: configPath, Executable: "/bin/mole"}
	var out bytes.Buffer
	require.NoError(t, waitUntilReady(&out, spec))
	assert.Greater(t, atomic.LoadInt32(&readyHits), int32(0), "readiness endpoint was not reached")
}

func TestPrepareEnvInitializesThenReuses(t *testing.T) {
	home := t.TempDir()
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}
	envPath := filepath.Join(home, ".env")

	var out bytes.Buffer
	require.NoError(t, prepareEnv(&out, spec))
	assert.Contains(t, out.String(), "Initialized environment file")
	assert.Contains(t, out.String(), envPath)
	_, err := os.Stat(envPath)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(envPath, []byte("KEEP=1\n"), 0o600))
	out.Reset()
	require.NoError(t, prepareEnv(&out, spec))
	assert.Contains(t, out.String(), "Using existing environment file")
	assert.Contains(t, out.String(), envPath)
	assert.Contains(t, out.String(), "mole service env refresh")
	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, "KEEP=1\n", string(data), "existing .env must not be overwritten")
}

func TestInstallNowRefusesManualInstance(t *testing.T) {
	home := t.TempDir()
	// Install now performs legacy autostart cleanup against the user home.
	t.Setenv("HOME", t.TempDir())
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}

	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	fake := &fakeService{status: managersvc.StatusStopped}
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: fake}, nil
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pid.RemovePidFile(home) })

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"install", "--now"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop that instance first")
	assert.Contains(t, err.Error(), strconv.Itoa(os.Getpid()))
	assert.Equal(t, 1, fake.installCalls)
	assert.Equal(t, 0, fake.startCalls, "manual instance must block the start")
}

func TestReinstallRefusesManualInstance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	spec := svc.Spec{HomePath: home, ConfigPath: filepath.Join(home, "config.json"), Executable: "/bin/mole"}

	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	fake := &fakeService{status: managersvc.StatusStopped}
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: fake}, nil
	}
	if _, err := pid.WritePidFile(home, "127.0.0.1", 18790); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pid.RemovePidFile(home) })

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"reinstall"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop that instance first")
	assert.Contains(t, err.Error(), strconv.Itoa(os.Getpid()))
	assert.NotContains(t, fake.calls, "stop")
	assert.NotContains(t, fake.calls, "uninstall")
	assert.NotContains(t, fake.calls, "install")
}

// stubGoBuildManager points the command at a go-run style executable and
// returns the fake service so tests can assert no lifecycle call happened.
func stubGoBuildManager(t *testing.T) (svc.Spec, *fakeService) {
	t.Helper()
	home := t.TempDir()
	spec := svc.Spec{
		HomePath:   home,
		ConfigPath: filepath.Join(home, "config.json"),
		Executable: "/tmp/go-build123/b001/exe/mole",
	}
	origSpec, origManager := resolveSpec, newManager
	t.Cleanup(func() { resolveSpec, newManager = origSpec, origManager })
	resolveSpec = func() (svc.Spec, error) { return spec, nil }
	fake := &fakeService{status: managersvc.StatusRunning}
	newManager = func(s svc.Spec, _ string) (*svc.Manager, error) {
		return &svc.Manager{Spec: s, Service: fake}, nil
	}
	return spec, fake
}

func TestReinstallRejectsGoBuildBeforeLifecycle(t *testing.T) {
	spec, fake := stubGoBuildManager(t)

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"reinstall"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go-build")
	assert.Empty(t, fake.calls, "reinstall must not stop, uninstall, install, or start")
	_, statErr := os.Stat(spec.EnvPath())
	assert.True(t, os.IsNotExist(statErr), "reinstall must fail before creating .env")
}

func TestInstallRejectsGoBuildBeforeEnv(t *testing.T) {
	spec, fake := stubGoBuildManager(t)

	cmd := NewServiceCommand()
	cmd.SetArgs([]string{"install"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go-build")
	assert.Empty(t, fake.calls, "install must not reach the platform service")
	_, statErr := os.Stat(spec.EnvPath())
	assert.True(t, os.IsNotExist(statErr), "failed install must not create .env")
}
