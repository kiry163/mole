// Mole - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kiry163/mole/cmd/mole/internal"
	"github.com/kiry163/mole/cmd/mole/internal/cliui"
	"github.com/kiry163/mole/cmd/mole/internal/serve"
	"github.com/kiry163/mole/cmd/mole/internal/service"
	"github.com/kiry163/mole/cmd/mole/internal/version"
	"github.com/kiry163/mole/pkg/config"
)

// NewMoleCommand returns the mole root command: bare `mole` prints help, and
// long-running work lives under explicit subcommands.
func NewMoleCommand() *cobra.Command {
	short := fmt.Sprintf("%s Mole — personal AI assistant", internal.Logo)
	long := fmt.Sprintf(`%s Mole is a lightweight personal AI assistant.

Version: %s`, internal.Logo, config.FormatVersion())

	cmd := &cobra.Command{
		Use:           "mole",
		Short:         short,
		Long:          long,
		Example:       "mole serve\n  mole version",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		fmt.Fprint(c.OutOrStdout(), cliui.RenderCommandHelp(c))
	})
	cmd.AddCommand(serve.NewServeCommand(), service.NewServiceCommand(), version.NewVersionCommand())
	return cmd
}

func main() {
	cmd := NewMoleCommand()
	last, err := cmd.ExecuteC()
	if err != nil {
		fmt.Fprint(os.Stderr, cliui.FormatCLIError(err.Error(), last))
		os.Exit(1)
	}
}
