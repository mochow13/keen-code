package config

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

var loaderLog = slog.New(slog.NewTextHandler(os.Stderr, nil))

type Loader struct{}

func NewLoader() *Loader {
	return &Loader{}
}

func (l *Loader) Load() (*GlobalConfig, error) {
	cfg := DefaultGlobalConfig()

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			loaderLog.Info("config file not found, using defaults")
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	loaderLog.Info("config loaded", "provider", cfg.ActiveProvider)
	return cfg, nil
}

func (l *Loader) Save(cfg *GlobalConfig) error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path := ConfigPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	loaderLog.Info("config saved", "path", path)
	return nil
}

func (l *Loader) Exists() bool {
	_, err := os.Stat(ConfigPath())
	return !os.IsNotExist(err)
}
