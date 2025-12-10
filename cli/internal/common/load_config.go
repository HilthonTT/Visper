package common

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"

	"github.com/hilthontt/visper/cli/config"
	"github.com/hilthontt/visper/cli/internal/utils"
	"github.com/pelletier/go-toml/v2"
)

func LoadConfigFile() {
	err := utils.LoadTomlFile(config.VisperMainDir, ConfigTomlString, &Config, config.FixConfigFile, false)
	if err != nil {
		userMsg := fmt.Sprintf("%s%s", LipglossError, err.Error())

		toExit := true
		var loadError *utils.TomlLoadError
		if errors.As(err, &loadError) && loadError != nil {
			if loadError.MissingFields() && !config.FixConfigFile {
				// Had missing fields and we did not fix
				userMsg += "\nTo add missing fields to configuration file automatically run superfile " +
					"with the --fix-config-file flag `spf --fix-config-file`"
			}
			toExit = loadError.IsFatal()
		}
		if toExit {
			utils.PrintfAndExitf("%s\n", userMsg)
		} else {
			fmt.Println(userMsg)
		}
	}

	// Even if there is a missing field, we want to validate fields that are present
	if err := ValidateConfig(&Config); err != nil {
		// If config is incorrect we cannot continue. We need to exit
		utils.PrintlnAndExit(err.Error())
	}
}

func ValidateConfig(c *ConfigType) error {

	return nil
}

func LoadHotkeysFile(ignoreMissingFields bool) {
	err := utils.LoadTomlFile(
		config.HotkeysFile,
		HotkeysTomlString,
		&Hotkeys,
		config.FixHotkeys,
		ignoreMissingFields,
	)
	if err != nil {
		userMsg := fmt.Sprintf("%s%s", LipglossError, err.Error())

		toExit := true
		var loadError *utils.TomlLoadError
		if errors.As(err, &loadError) {
			if loadError.MissingFields() && !config.FixHotkeys {
				// Had missing fields and we did not fix
				userMsg += "\nTo add missing fields to hotkeys file automatically run superfile " +
					"with the --fix-hotkeys flag `spf --fix-hotkeys`"
			}
			toExit = loadError.IsFatal()
		}
		if toExit {
			utils.PrintfAndExitf("%s\n", userMsg)
		} else {
			fmt.Println(userMsg)
		}
	}

	// Validate hotkey values
	val := reflect.ValueOf(Hotkeys)
	for i := range val.NumField() {
		field := val.Type().Field(i)
		value := val.Field(i)

		// Although this is redundant as Hotkey is always a slice
		// This adds a layer against accidental struct modifications
		// Makes sure its always be a string slice. It's somewhat like a unit test
		if value.Kind() != reflect.Slice || value.Type().Elem().Kind() != reflect.String {
			utils.PrintlnAndExit(LoadHotkeysError(field.Name))
		}

		hotkeysList, ok := value.Interface().([]string)
		if !ok || len(hotkeysList) == 0 || hotkeysList[0] == "" {
			utils.PrintlnAndExit(LoadHotkeysError(field.Name))
		}
	}
}

func LoadThemeFile() {
	themeFile := filepath.Join(config.ThemeFolder, Config.Theme+".toml")
	data, err := os.ReadFile(themeFile)
	if err == nil {
		unmarshalErr := toml.Unmarshal(data, &Theme)
		if unmarshalErr == nil {
			return
		}
		slog.Error("Could not unmarshal theme file. Falling back to default theme",
			"unmarshalErr", unmarshalErr)
	} else {
		slog.Error("Could not read user's theme file. Falling back to default theme", "path", themeFile, "error", err)
	}

	err = toml.Unmarshal([]byte(DefaultThemeString), &Theme)
	if err != nil {
		utils.PrintfAndExitf("Unexpected error while reading default theme file : %v. Exiting...", err)
	}
}

func LoadConfigStringGlobals(content embed.FS) error {
	hotkeyData, err := content.ReadFile(config.EmbedHotkeysFile)
	if err != nil {
		return err
	}
	HotkeysTomlString = string(hotkeyData)

	configData, err := content.ReadFile(config.EmbedConfigFile)
	if err != nil {
		return err
	}
	ConfigTomlString = string(configData)

	themeData, err := content.ReadFile(config.EmbedThemeCatppuccinFile)
	if err != nil {
		return err
	}
	DefaultThemeString = string(themeData)

	return nil
}

func LoadAllDefaultConfig(content embed.FS) {
	err := LoadConfigStringGlobals(content)
	if err != nil {
		slog.Error("Could not load default config from embed FS", "error", err)
		return
	}

	if err = os.MkdirAll(filepath.Dir(config.ThemeFileVersion), 0o755); err != nil {
		slog.Error("Error creating theme file parent directory", "error", err)
		return
	}

	err = os.WriteFile(config.ThemeFileVersion, []byte(config.CurrentVersion), 0o644)
	if err != nil {
		slog.Error("Error writing theme file version", "error", err)
	}
}
