package cliui

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// showErrHint
// ---------------------------------------------------------------------------

func TestShowErrHint(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		// Cobra flag errors — should show hint
		{"unknown flag: --foo", true},
		{"unknown shorthand flag: 'f' in -f", true},
		{"flag needs an argument: --output", true},
		{"required flag(s) \"model\" not set", true},
		// Generic invalid-argument errors — should show hint
		{"invalid argument \"abc\" for --count", true},
		// usage: in message — should show hint
		{"bad input\nusage: mole ...", true},
		// Should NOT false-positive on broad words
		{"connection flagged by remote", false},
		{"feature flag not set", false},
		{"invalid API key provided", false},
		{"authentication required", false},
		// Unrelated messages — no hint
		{"something went wrong", false},
	}

	for _, tc := range cases {
		if got := showErrHint(tc.msg); got != tc.want {
			t.Errorf("showErrHint(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestFormatCLIErrorIsPlainAndHintsAtHelp(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "mole serve",
		Short: "Run the gateway and web console in the foreground",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	cmd.Flags().String("port", "", "listen port")

	out := FormatCLIError("unknown flag: --nope", cmd)

	if !strings.HasPrefix(out, "Error: unknown flag: --nope\n") {
		t.Fatalf("error output should start with the message, got %q", out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("flag errors should include the usage text, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("output should carry no ANSI escapes, got %q", out)
	}
}

func TestFormatCLIErrorOmitsUsageForPlainFailures(t *testing.T) {
	cmd := &cobra.Command{Use: "mole serve"}
	cmd.Flags().String("port", "", "listen port")

	out := FormatCLIError("port 8080 is already in use", cmd)

	if out != "Error: port 8080 is already in use\n" {
		t.Fatalf("plain failure should not append usage, got %q", out)
	}
}
