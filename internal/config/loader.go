package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type Loader struct {
	path string
}

func NewLoader() *Loader {
	return &Loader{}
}

func NewLoaderWithPath(path string) *Loader {
	return &Loader{path: path}
}

func (l *Loader) configPath() string {
	if l.path != "" {
		return l.path
	}
	return ConfigPath()
}

func (l *Loader) configFixHint() string {
	if l.path != "" {
		return fmt.Sprintf("To fix configs manually, check %s", l.path)
	}
	return ConfigFixHint
}

func (l *Loader) Load() (*GlobalConfig, error) {
	cfg := DefaultGlobalConfig()

	data, err := os.ReadFile(l.configPath())
	if err != nil {
		if os.IsNotExist(err) && l.path == "" {
			slog.Debug("config file not found, using defaults")
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w. %s", err, l.configFixHint())
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w. %s", err, l.configFixHint())
	}

	slog.Debug("config loaded", "provider", cfg.ActiveProvider)
	return cfg, nil
}

func (l *Loader) Save(cfg *GlobalConfig) error {
	path := l.configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("failed to set config permissions: %w", err)
	}

	slog.Debug("config saved", "path", path)
	return nil
}

func (l *Loader) Exists() bool {
	_, err := os.Stat(l.configPath())
	return !os.IsNotExist(err)
}
