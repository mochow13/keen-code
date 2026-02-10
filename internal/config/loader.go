package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	viper *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(ConfigDir())

	defaults := DefaultGlobalConfig()
	v.SetDefault("provider", defaults.ActiveProvider)

	return &Loader{viper: v}
}

func (l *Loader) Load() (*GlobalConfig, error) {
	cfg := DefaultGlobalConfig()

	if err := l.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		return cfg, nil
	}

	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

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

	return nil
}

func (l *Loader) Exists() bool {
	_, err := os.Stat(ConfigPath())
	return !os.IsNotExist(err)
}
