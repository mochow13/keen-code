package providers

import (
	"embed"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryFS embed.FS

type Registry struct {
	Providers []Provider `yaml:"providers"`
}

type Provider struct {
	ID     string  `yaml:"id"`
	Name   string  `yaml:"name"`
	Models []Model `yaml:"models"`
}

type Model struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

func Load() (*Registry, error) {
	data, err := registryFS.ReadFile("registry.yaml")
	if err != nil {
		return nil, err
	}

	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

func (r *Registry) GetProvider(id string) (Provider, bool) {
	for _, p := range r.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
