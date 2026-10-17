//go:build !windows

package utils

import "os/exec"

// ExecCommand creates an exec.Cmd. On non-Windows platforms, this is
// a simple wrapper around exec.Command.
func ExecCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// ApplyProcAttrs is a no-op on non-Windows platforms.
func ApplyProcAttrs(_ *exec.Cmd) {}
