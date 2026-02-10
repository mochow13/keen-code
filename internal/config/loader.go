package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Loader handles loading and saving the global configuration.
type Loader struct {
	viper *viper.Viper
}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(ConfigDir())

	// Set defaults
	defaults := DefaultGlobalConfig()
	v.SetDefault("provider", defaults.ActiveProvider)

	return &Loader{viper: v}
}

// Load loads the global configuration from disk.
// If the config file doesn't exist, it returns a default config.
func (l *Loader) Load() (*GlobalConfig, error) {
	cfg := DefaultGlobalConfig()

	// Try to read existing config
	if err := l.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		// Config file not found, use defaults
		return cfg, nil
	}

	// Unmarshal into our struct
	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// Save persists the global configuration to disk.
func (l *Loader) Save(cfg *GlobalConfig) error {
	// Ensure config directory exists
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write with restricted permissions (user only)
	path := ConfigPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Exists checks if the config file exists.
func (l *Loader) Exists() bool {
	_, err := os.Stat(ConfigPath())
	return !os.IsNotExist(err)
}
