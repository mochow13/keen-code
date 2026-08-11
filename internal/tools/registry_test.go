package tools

import (
	"context"
	"testing"
)

func TestRegistryWithout_RemovesNamedToolsFromCopy(t *testing.T) {
	registry := NewRegistry()
	for _, name := range []string{"read_file", "write_file", "edit_file", "bash"} {
		if err := registry.Register(&dummyRegistryTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	filtered := registry.Without("write_file", "edit_file")

	if filtered.Count() != 2 {
		t.Fatalf("expected 2 tools, got %d", filtered.Count())
	}
	for _, name := range []string{"read_file", "bash"} {
		if _, ok := filtered.Get(name); !ok {
			t.Fatalf("expected %s to remain", name)
		}
	}
	for _, name := range []string{"write_file", "edit_file"} {
		if _, ok := filtered.Get(name); ok {
			t.Fatalf("expected %s to be removed", name)
		}
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("expected original registry to keep %s", name)
		}
	}
}

func TestRegistryAll_ReturnsToolsSortedByName(t *testing.T) {
	registry := NewRegistry()
	for _, name := range []string{"write_file", "bash", "read_file", "edit_file"} {
		if err := registry.Register(&dummyRegistryTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	got := registry.All()
	want := []string{"bash", "edit_file", "read_file", "write_file"}
	if len(got) != len(want) {
		t.Fatalf("expected %d tools, got %d", len(want), len(got))
	}
	for i, tool := range got {
		if tool.Name() != want[i] {
			t.Fatalf("tool %d: expected %q, got %q", i, want[i], tool.Name())
		}
	}
}

func TestRegistryRegisterRejectsInvalidTools(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(nil); err == nil {
		t.Fatal("Register(nil) returned nil error")
	}
	if err := registry.Register(&dummyRegistryTool{}); err == nil {
		t.Fatal("Register() accepted an empty name")
	}
	tool := &dummyRegistryTool{name: "read_file"}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(tool); err == nil {
		t.Fatal("Register() accepted a duplicate name")
	}
}

func TestValidateInput(t *testing.T) {
	input := map[string]any{"path": "file.txt"}
	if err := ValidateInput(context.Background(), &dummyRegistryTool{}, input); err != nil {
		t.Fatalf("ValidateInput() without validator error = %v", err)
	}

	want := context.Canceled
	tool := validatingRegistryTool{dummyRegistryTool: dummyRegistryTool{name: "read_file"}, validate: func(ctx context.Context, got any) error {
		gotInput, ok := got.(map[string]any)
		if ctx == nil || !ok || gotInput["path"] != input["path"] {
			t.Fatal("ValidateInput() did not pass context and input")
		}
		return want
	}}
	if err := ValidateInput(context.Background(), &tool, input); err != want {
		t.Fatalf("ValidateInput() error = %v, want %v", err, want)
	}
}

func TestMissingRequiredParameter(t *testing.T) {
	err := missingRequiredParameter("read_file", "path", `{ "path": "file.txt" }`, "Use a relative path.")
	if got, want := err.Error(), `invalid input: missing required "path" parameter. Retry read_file with all required fields: { "path": "file.txt" }. Use a relative path.`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if got := missingRequiredParameter("read_file", "path", "{}", "").Error(); got != `invalid input: missing required "path" parameter. Retry read_file with all required fields: {}` {
		t.Fatalf("error without guidance = %q", got)
	}
}

type dummyRegistryTool struct {
	name        string
	description string
	schema      map[string]any
	executed    bool
}

func (d *dummyRegistryTool) Name() string { return d.name }

func (d *dummyRegistryTool) Description() string { return d.description }

func (d *dummyRegistryTool) InputSchema() map[string]any { return d.schema }

func (d *dummyRegistryTool) Execute(ctx context.Context, input any) (any, error) {
	d.executed = true
	return map[string]any{"executed": true}, nil
}

type validatingRegistryTool struct {
	dummyRegistryTool
	validate func(context.Context, any) error
}

func (d validatingRegistryTool) ValidateInput(ctx context.Context, input any) error {
	return d.validate(ctx, input)
}
