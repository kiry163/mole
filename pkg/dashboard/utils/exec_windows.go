//go:build windows

package utils

import (
	"os/exec"
	"syscall"
)

// ExecCommand creates an exec.Cmd with the HideWindow attribute set
// to prevent console window flashes on Windows.
func ExecCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	ApplyProcAttrs(cmd)
	return cmd
}

// ApplyProcAttrs applies Windows-specific process attributes to hide
// the console window. It is safe to call with a nil cmd.
func ApplyProcAttrs(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
