package decision

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryFS embed.FS

type ProviderFactory interface {
	ID() string
	New(rawConfig json.RawMessage) (Evaluator, error)
}

type Registry struct {
	Providers []Provider `yaml:"providers"`
	factories map[string]ProviderFactory
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

// Load reads the supported decision-provider and model catalog. Factories are
// matched to catalog providers so unsupported provider IDs cannot be constructed.
func Load(factories ...ProviderFactory) (*Registry, error) {
	data, err := registryFS.ReadFile("registry.yaml")
	if err != nil {
		return nil, err
	}

	registry := &Registry{}
	if err := yaml.Unmarshal(data, registry); err != nil {
		return nil, fmt.Errorf("decode decision registry: %w", err)
	}
	if err := registry.validate(); err != nil {
		return nil, err
	}

	registry.factories = make(map[string]ProviderFactory, len(factories))
	for _, factory := range factories {
		if factory == nil || strings.TrimSpace(factory.ID()) == "" {
			return nil, fmt.Errorf("invalid decision provider factory")
		}
		if _, ok := registry.GetProvider(factory.ID()); !ok {
			return nil, fmt.Errorf("decision provider factory %q is not in registry", factory.ID())
		}
		if _, exists := registry.factories[factory.ID()]; exists {
			return nil, fmt.Errorf("duplicate decision provider factory %q", factory.ID())
		}
		registry.factories[factory.ID()] = factory
	}
	return registry, nil
}

func (r *Registry) GetProvider(id string) (Provider, bool) {
	for _, provider := range r.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

func (r *Registry) GetModel(providerID, modelID string) (Model, bool) {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return Model{}, false
	}
	for _, model := range provider.Models {
		if model.ID == modelID {
			return model, true
		}
	}
	return Model{}, false
}

func (r *Registry) New(provider string, rawConfig json.RawMessage) (Evaluator, error) {
	if r == nil {
		return nil, fmt.Errorf("decision provider registry is not initialized")
	}
	factory, ok := r.factories[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported decision provider %q", provider)
	}
	return factory.New(rawConfig)
}

func (r *Registry) validate() error {
	if len(r.Providers) == 0 {
		return fmt.Errorf("decision registry has no providers")
	}
	providers := make(map[string]struct{}, len(r.Providers))
	for _, provider := range r.Providers {
		if strings.TrimSpace(provider.ID) == "" || strings.TrimSpace(provider.Name) == "" {
			return fmt.Errorf("decision registry has provider with missing ID or name")
		}
		if _, exists := providers[provider.ID]; exists {
			return fmt.Errorf("duplicate decision provider %q", provider.ID)
		}
		providers[provider.ID] = struct{}{}
		if len(provider.Models) == 0 {
			return fmt.Errorf("decision provider %q has no models", provider.ID)
		}
		models := make(map[string]struct{}, len(provider.Models))
		for _, model := range provider.Models {
			if strings.TrimSpace(model.ID) == "" || strings.TrimSpace(model.Name) == "" {
				return fmt.Errorf("decision provider %q has model with missing ID or name", provider.ID)
			}
			if _, exists := models[model.ID]; exists {
				return fmt.Errorf("duplicate decision model %q for provider %q", model.ID, provider.ID)
			}
			models[model.ID] = struct{}{}
		}
	}
	return nil
}
