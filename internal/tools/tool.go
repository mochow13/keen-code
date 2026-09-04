package tools

import (
	"context"
	"fmt"
	"sort"
)

const (
	AskUserToolName   = "ask_user"
	BashToolName      = "bash"
	CallMCPToolName   = "call_mcp_tool"
	DelegateToolName  = "delegate_task"
	EditFileToolName  = "edit_file"
	GlobToolName      = "glob"
	GrepToolName      = "grep"
	ReadFileToolName  = "read_file"
	WebFetchToolName  = "web_fetch"
	WriteFileToolName = "write_file"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input any) (any, error)
}

type InputValidator interface {
	ValidateInput(ctx context.Context, input any) error
}

func ValidateInput(ctx context.Context, tool Tool, input any) error {
	validator, ok := tool.(InputValidator)
	if !ok {
		return nil
	}
	return validator.ValidateInput(ctx, input)
}

func missingRequiredParameter(tool, parameter, example, guidance string) error {
	message := fmt.Sprintf("invalid input: missing required %q parameter. Retry %s with all required fields: %s", parameter, tool, example)
	if guidance != "" {
		message += ". " + guidance
	}
	return fmt.Errorf("%s", message)
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
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})
	return all
}

func (r *Registry) Without(names ...string) *Registry {
	filtered := NewRegistry()
	excluded := make(map[string]struct{}, len(names))
	for _, name := range names {
		excluded[name] = struct{}{}
	}
	for name, tool := range r.tools {
		if _, ok := excluded[name]; ok {
			continue
		}
		filtered.tools[name] = tool
	}
	return filtered
}

func (r *Registry) Count() int {
	return len(r.tools)
}
