package utils

import (
	"github.com/kiry163/mole/pkg/config"
)

// GetMoleHome returns the mole home directory.
// Priority: $MOLE_HOME > ~/.mole
func GetMoleHome() string {
	return config.GetHome()
}
