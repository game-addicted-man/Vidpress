//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"vidpress/engine"
)

// Config is persisted to %APPDATA%\VidPress\settings.json.
type Config struct {
	Version    int             `json:"version"`
	Settings   engine.Settings `json:"settings"`
	LastAddDir string          `json:"lastAddDir"`
}

func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "VidPress")
}

func configPath() string { return filepath.Join(configDir(), "settings.json") }

func loadConfig() *Config {
	c := &Config{Version: 1, Settings: engine.DefaultSettings()}
	data, err := os.ReadFile(configPath())
	if err == nil {
		var loaded Config
		if json.Unmarshal(data, &loaded) == nil && loaded.Version == 1 {
			loaded.Settings.Normalize()
			c = &loaded
		}
	}
	if c.Settings.OutputDir != "" {
		if st, err := os.Stat(c.Settings.OutputDir); err != nil || !st.IsDir() {
			c.Settings.OutputDir = ""
		}
	}
	return c
}

func (c *Config) Save() error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}
