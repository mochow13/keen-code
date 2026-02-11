package providers

import (
	"embed"

	"github.com/charmbracelet/huh"
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

func (r *Registry) GetModel(providerID, modelID string) (Model, bool) {
	p, ok := r.GetProvider(providerID)
	if !ok {
		return Model{}, false
	}
	for _, m := range p.Models {
		if m.ID == modelID {
			return m, true
		}
	}
	return Model{}, false
}

func (r *Registry) ProviderOptions() []huh.Option[string] {
	opts := make([]huh.Option[string], len(r.Providers))
	for i, p := range r.Providers {
		opts[i] = huh.NewOption(p.Name, p.ID)
	}
	return opts
}

func (r *Registry) ModelOptions(providerID string) []huh.Option[string] {
	p, ok := r.GetProvider(providerID)
	if !ok {
		return nil
	}
	opts := make([]huh.Option[string], len(p.Models))
	for i, m := range p.Models {
		opts[i] = huh.NewOption(m.Name, m.ID)
	}
	return opts
}
