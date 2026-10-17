package cliui

import "github.com/spf13/cobra"

// RenderCommandHelp renders the help text for a command.
func RenderCommandHelp(c *cobra.Command) string {
	return plainCommandHelp(c)
}
