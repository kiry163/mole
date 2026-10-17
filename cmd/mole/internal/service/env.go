// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	svc "github.com/kiry163/mole/pkg/service"
)

func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage the service environment file",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.AddCommand(newEnvRefreshCommand())
	return cmd
}

func newEnvRefreshCommand() *cobra.Command {
	var (
		restart bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Import shell settings into the instance .env file",
		Long: `Import PATH, locale, timezone, proxy, and TLS settings from the current
shell into the instance .env file. Other lines are preserved.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if restart && dryRun {
				return errors.New("--restart cannot be combined with --dry-run")
			}
			spec, err := resolveSpec()
			if err != nil {
				return err
			}
			updates, err := svc.RefreshEnv(spec.ConfigPath, dryRun)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(updates) == 0 {
				fmt.Fprintln(w, "No environment values imported")
			} else {
				keys := make([]string, 0, len(updates))
				for key := range updates {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					fmt.Fprintf(w, "%s=%s\n", key, updates[key])
				}
			}
			if dryRun {
				fmt.Fprintf(w, "Dry run: %s was not modified\n", spec.EnvPath())
				return nil
			}
			fmt.Fprintf(w, "Updated %s\n", spec.EnvPath())
			if !restart {
				return nil
			}

			m, err := newManager(spec, "")
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
	cmd.Flags().BoolVar(&restart, "restart", false, "Restart the service after refreshing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show changes without writing the file")
	return cmd
}
