package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

const (
	TransportStreamableHTTP = "streamable_http"
	TransportStdio          = "stdio"

	AuthNone   = "none"
	AuthAPIKey = "api_key"
	AuthOAuth  = "oauth"
)

var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

type ServerConfig struct {
	URL     string            `json:"url,omitempty"`
	Auth    AuthConfig        `json:"auth,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type AuthConfig struct {
	Type   string   `json:"type,omitempty"`
	Header string   `json:"header,omitempty"`
	Scheme string   `json:"scheme,omitempty"`
	Key    string   `json:"key,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".keen", "mcp", "configs.json")
}

func LoadConfigWithPath(path string) (*Config, error) {
	useDefault := path == ""
	if useDefault {
		path = DefaultConfigPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if useDefault && errors.Is(err, os.ErrNotExist) {
			return &Config{Servers: map[string]ServerConfig{}}, nil
		}
		return nil, fmt.Errorf("load MCP config %q: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse MCP config: %w", err)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	for name, server := range c.Servers {
		if err := validateServerName(name); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
		if err := server.validate(); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
	}
	return nil
}

func validateServerName(name string) error {
	if !serverNamePattern.MatchString(name) {
		return errors.New("name must be 1-128 characters and contain only letters, numbers, underscores, dashes, and dots")
	}
	return nil
}

func (s ServerConfig) validate() error {
	if s.Command != "" {
		if s.Auth.Type != "" && s.Auth.Type != AuthNone {
			return errors.New("stdio transport does not support HTTP auth")
		}
		return nil
	}
	if err := validateHTTPURL(s.URL); err != nil {
		return err
	}
	return s.Auth.withDefaults().validateHTTP()
}

func validateHTTPURL(raw string) error {
	if raw == "" {
		return errors.New("streamable HTTP requires url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if u.Host == "" {
		return errors.New("url must include host")
	}
	return nil
}

func (a AuthConfig) withDefaults() AuthConfig {
	if a.Type == "" {
		a.Type = AuthNone
	}
	if a.Type == AuthAPIKey {
		headerProvided := a.Header != ""
		if a.Header == "" {
			a.Header = "Authorization"
		}
		if a.Scheme == "" && !headerProvided {
			a.Scheme = "Bearer"
		}
	}
	return a
}

func (a AuthConfig) validateHTTP() error {
	switch a.Type {
	case "", AuthNone:
		return nil
	case AuthAPIKey:
		if a.Key == "" {
			return errors.New("api_key auth requires key")
		}
		if a.Header == "" {
			return errors.New("api_key auth requires header")
		}
		return nil
	case AuthOAuth:
		return nil
	default:
		return fmt.Errorf("unsupported auth type %q", a.Type)
	}
}
