package providers

import (
	"embed"
	"fmt"
	"slices"
	"strings"

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
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	ContextWindow   int      `yaml:"context_window"`
	ThinkingEfforts []string `yaml:"thinking_efforts"`
}

func (m Model) SupportsThinkingEffort() bool {
	return len(m.ThinkingEfforts) > 0
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

func ResolveThinkingEffort(provider, model, effort string) (string, error) {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return "", nil
	}

	registry, err := Load()
	if err != nil {
		return "", fmt.Errorf("load provider registry: %w", err)
	}
	modelMeta, ok := registry.GetModel(provider, model)
	if !ok {
		return effort, nil
	}
	if !slices.Contains(modelMeta.ThinkingEfforts, effort) {
		if len(modelMeta.ThinkingEfforts) == 0 {
			return "", fmt.Errorf("model %s/%s does not support configurable thinking", provider, model)
		}
		return "", fmt.Errorf("unsupported thinking effort %q for %s/%s; expected one of: %s", effort, provider, model, strings.Join(modelMeta.ThinkingEfforts, ", "))
	}
	return effort, nil
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

func (r *Registry) GetModelContextWindow(providerID, modelID string) (int, bool) {
	p, ok := r.GetProvider(providerID)
	if !ok {
		return 0, false
	}
	for _, m := range p.Models {
		if m.ID == modelID {
			if m.ContextWindow <= 0 {
				return 0, false
			}
			return m.ContextWindow, true
		}
	}
	return 0, false
}
