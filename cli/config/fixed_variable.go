package config

import (
	"path/filepath"

	"github.com/adrg/xdg"
)

const (
	CurrentVersion = "v1.0.0"

	EmbedConfigDir           = "src/visper_config"
	EmbedHotkeysFile         = EmbedConfigDir + "/hotkeys.toml"
	EmbedConfigFile          = EmbedConfigDir + "/config.toml"
	EmbedThemeDir            = EmbedConfigDir + "/theme"
	EmbedThemeCatppuccinFile = EmbedThemeDir + "/catppuccin.toml"
)

var (
	HomeDir       = xdg.Home
	VisperMainDir = filepath.Join(xdg.ConfigHome, "visper")
	VisperDataDir = filepath.Join(xdg.DataHome, "visper")

	// StateDir files
	LogFile = filepath.Join(VisperMainDir, "visper.log")

	// MainDir files
	ThemeFolder = filepath.Join(VisperMainDir, "theme")

	// DataDir files
	LastCheckVersion = filepath.Join(VisperDataDir, "lastCheckVersion")
	ThemeFileVersion = filepath.Join(VisperDataDir, "themeFileVersion")
)

// These variables are actually not fixed, they are sometimes updated dynamically
var (
	ConfigFile  = filepath.Join(VisperMainDir, "config.toml")
	HotkeysFile = filepath.Join(VisperMainDir, "hotkeys.toml")

	// Other state variables
	FixConfigFile = false
	FixHotkeys    = false
)
