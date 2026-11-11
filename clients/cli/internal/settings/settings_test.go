package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "visper")
	m := newManager(dir)

	if got := m.GetUserConfig().SelectedWaifu; got != 1 {
		t.Fatalf("default SelectedWaifu = %d, want 1", got)
	}

	if err := m.SetUserConfig(&UserConfig{SelectedWaifu: 2}); err != nil {
		t.Fatal(err)
	}
	if got := m.GetUserConfig().SelectedWaifu; got != 2 {
		t.Fatalf("cached SelectedWaifu = %d, want 2", got)
	}

	// A fresh manager must read the persisted value.
	if got := newManager(dir).GetUserConfig().SelectedWaifu; got != 2 {
		t.Fatalf("persisted SelectedWaifu = %d, want 2", got)
	}

	// Mutating the returned copy must not affect the cache.
	cfg := m.GetUserConfig()
	cfg.SelectedWaifu = 99
	if got := m.GetUserConfig().SelectedWaifu; got != 2 {
		t.Fatalf("cache was mutated through returned pointer: %d", got)
	}
}

func TestManagerCorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := newManager(dir).GetUserConfig().SelectedWaifu; got != 1 {
		t.Fatalf("SelectedWaifu = %d, want default 1", got)
	}
}

func TestConfigDirIsNotWorkingDirectory(t *testing.T) {
	if _, err := os.UserConfigDir(); err != nil {
		t.Skip("no user config dir on this system")
	}
	dir := ConfigDir()
	if dir == "." || filepath.Base(dir) != appName {
		t.Fatalf("ConfigDir() = %q", dir)
	}
}
