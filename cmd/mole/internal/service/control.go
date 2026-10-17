// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"errors"
	"fmt"
	"io"
	"os"

	managersvc "github.com/kardianos/service"
	"github.com/spf13/cobra"

	svc "github.com/kiry163/mole/pkg/service"
)

func newInstallCommand() *cobra.Command {
	var now bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the service to start at login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := resolveSpec()
			if err != nil {
				return err
			}
			password, err := readPassword()
			if err != nil {
				return err
			}
			m, err := newManager(spec, password)
			if err != nil {
				return err
			}
			// Reject a temporary go-run executable before creating .env.
			if err := m.ValidateInstall(); err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			// The service runs with the manager's minimal environment, so
			// snapshot the current shell the first time. Existing files are
			// never overwritten.
			if err := prepareEnv(w, spec); err != nil {
				return err
			}
			if err := m.Install(); err != nil {
				if alreadyInstalled(err) {
					return errors.New(`service already installed; use "mole service reinstall"`)
				}
				return fmt.Errorf("install service: %w", err)
			}
			printInstalled(w, spec)
			cleanupLegacyAutostart(w)
			if now {
				return startInstalled(w, spec, m)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&now, "now", false, "Start the service after installing")
	return cmd
}

func newReinstallCommand() *cobra.Command {
	var now bool
	cmd := &cobra.Command{
		Use:   "reinstall",
		Short: "Replace the service definition, restoring the previous run state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := resolveSpec()
			if err != nil {
				return err
			}
			password, err := readPassword()
			if err != nil {
				return err
			}
			m, err := newManager(spec, password)
			if err != nil {
				return err
			}
			// Fail before stopping or uninstalling: a temporary go-run
			// executable cannot be reinstalled.
			if err := m.ValidateInstall(); err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if err := prepareEnv(w, spec); err != nil {
				return err
			}

			installed, running, err := serviceState(m)
			if err != nil {
				return err
			}
			if !running {
				if data := manualRunning(spec); data != nil {
					return manualInstanceError(data.PID)
				}
			}
			if installed && running {
				if err := m.Stop(); err != nil {
					return fmt.Errorf("stop service before reinstall: %w", err)
				}
				if err := waitStopped(spec); err != nil {
					return err
				}
			}
			if installed {
				if err := m.Uninstall(); err != nil {
					return fmt.Errorf("uninstall service: %w", err)
				}
			}
			if err := m.Install(); err != nil {
				return fmt.Errorf("install service: %w", err)
			}
			printInstalled(w, spec)
			cleanupLegacyAutostart(w)
			if running || now {
				return startInstalled(w, spec, m)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&now, "now", false, "Start the service after reinstalling")
	return cmd
}

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the service definition and stop the service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, m, err := loadManager("")
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			installed, running, err := serviceState(m)
			if err != nil {
				return err
			}
			if !running {
				if data := manualRunning(spec); data != nil {
					fmt.Fprintf(w, "Mole is running manually (PID %d); leaving it running\n", data.PID)
				}
			}
			if !installed {
				fmt.Fprintln(w, "Service is not installed")
				cleanupLegacyAutostart(w)
				return nil
			}
			if running {
				if err := stopService(w, m); err != nil {
					return err
				}
			}
			if err := m.Uninstall(); err != nil {
				if errors.Is(err, managersvc.ErrNotInstalled) {
					fmt.Fprintln(w, "Service is not installed")
					cleanupLegacyAutostart(w)
					return nil
				}
				return fmt.Errorf("uninstall service: %w", err)
			}
			fmt.Fprintf(w, "Uninstalled %s (data in %s was kept)\n", spec.Name(), spec.HomePath)
			cleanupLegacyAutostart(w)
			return nil
		},
	}
}

// cleanupLegacyAutostart removes autostart entries from older mole releases.
// Cleanup is best effort: problems are reported as warnings and never fail
// the uninstall.
func cleanupLegacyAutostart(w io.Writer) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(w, "Warning: could not locate home for legacy autostart cleanup: %v\n", err)
		return
	}
	for _, err := range svc.RemoveLegacyAutostart(home) {
		fmt.Fprintf(w, "Warning: could not remove legacy autostart entry: %v\n", err)
	}
}

func newStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the installed service and wait until it is ready",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, m, err := loadManager("")
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			// CollectStatus reads the config, which may contain $VAR
			// references resolved from .env.
			if err := svc.LoadEnv(spec.ConfigPath); err != nil {
				return err
			}
			st, err := m.CollectStatus()
			if err != nil {
				return fmt.Errorf("check service status: %w", err)
			}
			if !st.Installed {
				return errors.New(`service is not installed; run "mole service install" first`)
			}
			if st.ManualRunning {
				return manualInstanceError(st.PID)
			}
			if st.Running {
				fmt.Fprintf(w, "Service is already running at %s\n", st.URL)
				return nil
			}
			return startInstalled(w, spec, m)
		},
	}
}

func newStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the service without touching a manually started instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, m, err := loadManager("")
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			_, running, err := serviceState(m)
			if err != nil {
				return err
			}
			if data := manualRunning(spec); data != nil && !running {
				fmt.Fprintf(w, "Mole is running manually (PID %d); leaving it running. Stop it with: kill %d\n", data.PID, data.PID)
				return nil
			}
			if !running {
				fmt.Fprintln(w, "Service is already stopped")
				return nil
			}
			return stopService(w, m)
		},
	}
}

func newRestartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the service and wait until it is ready",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, m, err := loadManager("")
			if err != nil {
				return err
			}
			installed, running, err := serviceState(m)
			if err != nil {
				return err
			}
			if !installed {
				return errors.New(`service is not installed; run "mole service install" first`)
			}
			if !running {
				if data := manualRunning(spec); data != nil {
					return manualInstanceError(data.PID)
				}
			}
			w := cmd.OutOrStdout()
			fmt.Fprintln(w, "Restarting service...")
			if err := m.Restart(); err != nil {
				return fmt.Errorf("restart service: %w", err)
			}
			if err := waitUntilReady(w, spec); err != nil {
				return err
			}
			fmt.Fprintln(w, "Service is ready")
			return nil
		},
	}
}
