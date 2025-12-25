package settings_manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	appName    = "visper"
	configFile = "config.json"
)

type settingsManager struct {
	configPath string
	cache      *UserConfig
	mu         sync.RWMutex
}

func NewSettingsManager() SettingsManager {
	configDir := getConfigDir()
	configPath := filepath.Join(configDir, configFile)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create config directory: %v\n", err)
	}

	return &settingsManager{
		configPath: configPath,
	}
}

func (s *settingsManager) GetUserConfig() *UserConfig {
	s.mu.RLock()
	if s.cache != nil {
		defer s.mu.RUnlock()
		return s.cache
	}
	s.mu.RUnlock()

	// Cache miss - load from disk
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if s.cache != nil {
		return s.cache
	}

	defaultConfig := &UserConfig{
		SelectedWaifu: 1, // check waifus.json (first id is 1)
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig
		}
		return defaultConfig
	}

	var config UserConfig
	if err := json.Unmarshal(data, &config); err != nil {
		s.cache = defaultConfig
		return s.cache
	}

	s.cache = &config

	return &config
}

func (s *settingsManager) SetUserConfig(config *UserConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()

	return nil
}

// getConfigDir returns the appropriate configuration directory based on OS
func getConfigDir() string {
	var configDir string

	if os.Getenv("XDG_CONFIG_HOME") != "" {
		configDir = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), appName)
	} else if home, err := os.UserHomeDir(); err != nil {
		// Fallback to home directory
		switch {
		case fileExists(filepath.Join(home, ".config")):
			// Linux/Unix: ~/.config/visper
			configDir = filepath.Join(home, ".config", appName)
		default:
			// Windows: %USERPROFILE%\AppData\Roaming\visper
			// macOS: ~/Library/Application Support/visper
			configDir = filepath.Join(home, getOSSpecificDir(), appName)
		}
	} else {
		// Last resort: current directory
		configDir = "."
	}

	return configDir
}

func getOSSpecificDir() string {
	if os.Getenv("APPDATA") != "" {
		return filepath.Join("AppData", "Roaming")
	}

	return filepath.Join("Library", "Application Support")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
