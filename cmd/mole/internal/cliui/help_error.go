package cliui

import (
	"strings"

	"github.com/spf13/cobra"
)

// FormatCLIError formats an error for stderr. Flag/usage errors keep a short
// hint pointing at the full help instead of dumping the whole usage text.
func FormatCLIError(msg string, ctx *cobra.Command) string {
	msg = strings.TrimRight(msg, "\n")
	s := "Error: " + msg + "\n"
	if ctx == nil || !showErrHint(msg) {
		return s
	}
	return s + "\n" + plainCommandHelp(ctx)
}

func showErrHint(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "unknown flag") ||
		strings.Contains(m, "unknown shorthand flag") ||
		strings.Contains(m, "flag needs an argument") ||
		strings.Contains(m, "invalid argument") ||
		strings.Contains(m, "required flag") ||
		strings.Contains(m, "usage:")
}
