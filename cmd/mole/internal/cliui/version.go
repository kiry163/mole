package cliui

import "fmt"

// PrintVersion prints version, optional build info, and Go toolchain line.
func PrintVersion(logo, versionLine string, build, goVer string) {
	fmt.Printf("%s %s\n", logo, versionLine)
	if build != "" {
		fmt.Printf("  Build: %s\n", build)
	}
	if goVer != "" {
		fmt.Printf("  Go: %s\n", goVer)
	}
}
