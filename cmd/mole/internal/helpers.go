package internal

import (
	"os"
	"path/filepath"

	"github.com/kiry163/mole/pkg"
	"github.com/kiry163/mole/pkg/config"
)

const Logo = pkg.Logo

// GetMoleHome returns the mole home directory.
// Priority: $MOLE_HOME > ~/.mole
func GetMoleHome() string {
	return config.GetHome()
}

func GetConfigPath() string {
	if configPath := os.Getenv(config.EnvConfig); configPath != "" {
		return configPath
	}
	return filepath.Join(GetMoleHome(), "config.json")
}
