package version

import (
	"github.com/spf13/cobra"

	"github.com/kiry163/mole/cmd/mole/internal"
	"github.com/kiry163/mole/cmd/mole/internal/cliui"
	"github.com/kiry163/mole/pkg/config"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
		},
	}

	return cmd
}

func printVersion() {
	build, goVer := config.FormatBuildInfo()
	cliui.PrintVersion(internal.Logo, "mole "+config.FormatVersion(), build, goVer)
}
