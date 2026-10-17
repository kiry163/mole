// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	managersvc "github.com/kardianos/service"
	"github.com/spf13/cobra"

	"github.com/kiry163/mole/cmd/mole/internal"
	"github.com/kiry163/mole/pkg/pid"
	svc "github.com/kiry163/mole/pkg/service"
)

const (
	readyTimeout = 30 * time.Second
	stopTimeout  = 45 * time.Second
	logHintLines = 20
)

// resolveSpec and newManager are package variables so tests can exercise the
// command surface without consulting the host service manager.
var (
	resolveSpec = func() (svc.Spec, error) {
		return svc.NewSpec(internal.GetMoleHome(), internal.GetConfigPath())
	}
	newManager = svc.NewManager
)

// NewServiceCommand returns the `mole service` command tree. Bare
// `mole service` prints help; all lifecycle work lives in subcommands.
func NewServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the Mole background service",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.AddCommand(
		newInstallCommand(),
		newReinstallCommand(),
		newUninstallCommand(),
		newStartCommand(),
		newStopCommand(),
		newRestartCommand(),
		newStatusCommand(),
		newLogsCommand(),
		newEnvCommand(),
	)
	return cmd
}

func loadManager(password string) (svc.Spec, *svc.Manager, error) {
	spec, err := resolveSpec()
	if err != nil {
		return svc.Spec{}, nil, err
	}
	m, err := newManager(spec, password)
	if err != nil {
		return svc.Spec{}, nil, err
	}
	return spec, m, nil
}

// serviceState reports the installed/running state without probing the
// gateway or loading configuration. A missing unit is a valid state.
func serviceState(m *svc.Manager) (installed, running bool, err error) {
	st, err := m.Status()
	if errors.Is(err, managersvc.ErrNotInstalled) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("check service status: %w", err)
	}
	if st == managersvc.StatusUnknown {
		return false, false, nil
	}
	return true, st == managersvc.StatusRunning, nil
}

func printInstalled(w io.Writer, spec svc.Spec) {
	fmt.Fprintf(w, "Installed %s\n", spec.Name())
	if unit, err := spec.UnitPath(); err == nil && unit != "" {
		fmt.Fprintln(w, unit)
	}
}

func alreadyInstalled(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already installed")
}

func manualInstanceError(pidValue int) error {
	return fmt.Errorf("a manually started mole instance is running (PID %d); stop that instance first", pidValue)
}

// manualRunning returns the live PID file entry, if the instance was started
// outside the service manager.
func manualRunning(spec svc.Spec) *pid.PidFileData {
	return pid.ReadPidFileWithCheck(spec.HomePath)
}

// prepareEnv initializes the instance .env file from the current shell when it
// is missing and reports which case applied. Existing files are never
// overwritten.
func prepareEnv(w io.Writer, spec svc.Spec) error {
	path := spec.EnvPath()
	_, err := os.Stat(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := svc.EnsureEnv(spec.ConfigPath); err != nil {
		return fmt.Errorf("initialize %s: %w", path, err)
	}
	if existed {
		fmt.Fprintf(w, "Using existing environment file %s (run \"mole service env refresh\" to update it)\n", path)
	} else {
		fmt.Fprintf(w, "Initialized environment file %s from the current shell\n", path)
	}
	return nil
}

// startInstalled refuses to start while a manually started instance holds the
// home, then starts the service and waits for readiness.
func startInstalled(w io.Writer, spec svc.Spec, m *svc.Manager) error {
	if data := manualRunning(spec); data != nil {
		return manualInstanceError(data.PID)
	}
	return startAndWait(w, m)
}

// startAndWait starts the installed service and waits for the gateway to
// report readiness.
func startAndWait(w io.Writer, m *svc.Manager) error {
	fmt.Fprintln(w, "Starting service...")
	if err := m.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	if err := waitUntilReady(w, m.Spec); err != nil {
		return err
	}
	fmt.Fprintln(w, "Service is ready")
	return nil
}

// waitUntilReady waits for the gateway /ready endpoint. On failure it prints
// the recent service log so the terminal shows why startup failed.
func waitUntilReady(w io.Writer, spec svc.Spec) error {
	// WaitReady loads the config, which may contain $VAR references. The
	// service program applies .env before starting the gateway; commands that
	// only observe readiness must do the same.
	if err := svc.LoadEnv(spec.ConfigPath); err != nil {
		return err
	}
	if err := svc.WaitReady(spec, readyTimeout); err != nil {
		fmt.Fprintf(w, "Recent service logs (%d lines):\n", logHintLines)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		tailErr := svc.TailLogs(ctx, spec, w, logHintLines, false)
		cancel()
		if tailErr != nil {
			fmt.Fprintf(w, "(no service logs: %v)\n", tailErr)
		}
		return fmt.Errorf(
			"%w\nhint: check gateway.host and gateway.port in %s, then run \"mole service logs\"",
			err, spec.ConfigPath)
	}
	return nil
}

// stopService stops the installed service and waits for its PID file to
// disappear.
func stopService(w io.Writer, m *svc.Manager) error {
	if err := m.Stop(); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	if err := waitStopped(m.Spec); err != nil {
		return err
	}
	fmt.Fprintf(w, "Stopped %s\n", m.Spec.Name())
	return nil
}

func waitStopped(spec svc.Spec) error {
	deadline := time.Now().Add(stopTimeout)
	for {
		if pid.ReadPidFileWithCheck(spec.HomePath) == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf(
				"service did not stop within %s (PID file %s still present)",
				stopTimeout, pid.FilePath(spec.HomePath))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
