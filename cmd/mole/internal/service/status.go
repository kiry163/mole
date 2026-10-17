// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	svc "github.com/kiry163/mole/pkg/service"
)

func newStatusCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show service, process, gateway, and environment status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, m, err := loadManager("")
			if err != nil {
				return err
			}
			// Load .env so config values written as $VAR references resolve
			// the same way they do when the gateway starts. Status remains
			// useful when the file is missing or invalid.
			_ = svc.LoadEnv(spec.ConfigPath)

			st, err := m.CollectStatus()
			w := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				if encErr := enc.Encode(st); encErr != nil {
					return encErr
				}
			} else {
				printStatus(w, st)
			}
			if err != nil {
				return fmt.Errorf("service status: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output status as JSON")
	return cmd
}

func printStatus(w io.Writer, st svc.Status) {
	fmt.Fprintf(w, "Name:         %s\n", st.Name)
	fmt.Fprintf(w, "Display name: %s\n", st.DisplayName)
	fmt.Fprintf(w, "Home:         %s\n", st.Home)
	fmt.Fprintf(w, "Config:       %s\n", st.ConfigPath)
	fmt.Fprintf(w, "Executable:   %s\n", st.Executable)
	if st.UnitPath != "" {
		fmt.Fprintf(w, "Unit:         %s\n", st.UnitPath)
	}
	fmt.Fprintf(w, "Installed:    %s\n", yesNo(st.Installed))
	if st.ManualRunning {
		fmt.Fprintln(w, "Running:      yes (manual)")
	} else {
		fmt.Fprintf(w, "Running:      %s\n", yesNo(st.Running))
	}
	fmt.Fprintf(w, "Manual:       %s\n", yesNo(st.ManualRunning))
	if st.PID != 0 {
		fmt.Fprintf(w, "PID:          %d\n", st.PID)
	}
	if st.URL != "" {
		fmt.Fprintf(w, "URL:          %s\n", st.URL)
	}
	if st.Health != "" {
		fmt.Fprintf(w, "Health:       %s\n", st.Health)
	}
	if st.EnvChanged {
		fmt.Fprintln(w, `Env changed:  yes (run "mole service restart" to apply)`)
	} else {
		fmt.Fprintln(w, "Env changed:  no")
	}
	if st.ConfigError != "" {
		fmt.Fprintf(w, "Config error: %s\n", st.ConfigError)
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func newLogsCommand() *cobra.Command {
	var (
		follow bool
		lines  int
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent service logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := resolveSpec()
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := svc.TailLogs(ctx, spec, cmd.OutOrStdout(), lines, follow); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&lines, "lines", 200, "Number of recent log lines to show")
	return cmd
}
