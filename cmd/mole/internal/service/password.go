// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package service

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/term"
)

// readPassword prompts for the Windows account password required to register a
// current-user service. The value is handed to the service manager directly
// and never appears in argv. Other platforms do not need a password.
func readPassword() (string, error) {
	if runtime.GOOS != "windows" {
		return "", nil
	}
	fmt.Fprint(os.Stdout, "Windows account password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", fmt.Errorf("read service password: %w", err)
	}
	return string(password), nil
}
