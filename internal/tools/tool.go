package tools

import (
	"context"
	"fmt"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input any) (any, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("cannot register nil tool")
	}

	name := t.Name()
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q is already registered", name)
	}

	r.tools[name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, exists := r.tools[name]
	return t, exists
}

func (r *Registry) All() []Tool {
	all := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		all = append(all, t)
	}
	return all
}

func (r *Registry) Count() int {
	return len(r.tools)
}
