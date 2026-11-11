package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	appName    = "visper"
	configFile = "config.json"
)

type manager struct {
	configPath string
	cache      *UserConfig
	mu         sync.RWMutex
}

// NewManager returns a Manager that stores its configuration in
// <os.UserConfigDir()>/visper/config.json.
func NewManager() Manager {
	return newManager(ConfigDir())
}

func newManager(configDir string) *manager {
	return &manager{
		configPath: filepath.Join(configDir, configFile),
	}
}

// ConfigDir returns the per-user configuration directory for visper. It
// honours XDG_CONFIG_HOME on Unix, %AppData% on Windows and
// ~/Library/Application Support on macOS (see os.UserConfigDir). If no such
// directory can be determined it falls back to the current directory.
func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		slog.Warn("cannot determine user config directory, using working directory", "error", err)
		return "."
	}
	return filepath.Join(dir, appName)
}

func defaultConfig() *UserConfig {
	return &UserConfig{
		SelectedWaifu: 1, // check waifus.json (first id is 1)
	}
}

// GetUserConfig returns a copy of the current configuration, loading it from
// disk on first use. A missing or corrupt file yields the defaults.
func (s *manager) GetUserConfig() *UserConfig {
	s.mu.RLock()
	if s.cache != nil {
		cfg := *s.cache
		s.mu.RUnlock()
		return &cfg
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cache == nil {
		s.cache = s.load()
	}
	cfg := *s.cache
	return &cfg
}

func (s *manager) load() *UserConfig {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Error("failed to read config file", "path", s.configPath, "error", err)
		}
		return defaultConfig()
	}

	config := defaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		slog.Error("failed to parse config file", "path", s.configPath, "error", err)
		return defaultConfig()
	}
	return config
}

// SetUserConfig writes config to disk and updates the cached copy.
func (s *manager) SetUserConfig(config *UserConfig) error {
	if config == nil {
		return errors.New("config must not be nil")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.configPath), 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(s.configPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	cfg := *config
	s.cache = &cfg
	return nil
}
