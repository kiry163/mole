// Package cliui renders Mole's human-oriented CLI output as plain text.
//
// The CLI surface is `mole` (start the console), `mole version` and `--help`,
// so nothing here negotiates terminal width or ANSI styling: help, errors and
// version output stay plain and pipeable.
package cliui

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// plainCommandHelp renders Cobra's own usage text: description, usage line,
// aliases, flags and subcommands, with no styling.
func plainCommandHelp(c *cobra.Command) string {
	desc := c.Long
	if desc == "" {
		desc = c.Short
	}
	desc = strings.TrimRight(desc, " \t\n\r")
	var b strings.Builder
	if desc != "" {
		fmt.Fprintln(&b, desc)
		fmt.Fprintln(&b)
	}
	if c.Runnable() || c.HasSubCommands() {
		b.WriteString(c.UsageString())
	}
	return b.String()
}
