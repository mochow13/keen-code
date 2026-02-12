package providers

import (
	"testing"
)

func TestLoad(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if reg == nil {
		t.Fatal("Load() returned nil registry")
	}
	if len(reg.Providers) == 0 {
		t.Error("Load() returned empty providers list")
	}
}

func TestRegistry_GetProvider(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{ID: "anthropic", Name: "Anthropic"},
			{ID: "openai", Name: "OpenAI"},
		},
	}

	p, ok := reg.GetProvider("anthropic")
	if !ok {
		t.Error("GetProvider('anthropic') should return true")
	}
	if p.ID != "anthropic" || p.Name != "Anthropic" {
		t.Errorf("GetProvider returned wrong provider: %+v", p)
	}

	_, ok = reg.GetProvider("unknown")
	if ok {
		t.Error("GetProvider('unknown') should return false")
	}
}

func TestRegistry_GetModel(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{
				ID:   "anthropic",
				Name: "Anthropic",
				Models: []Model{
					{ID: "claude-3-opus", Name: "Claude 3 Opus"},
					{ID: "claude-3-sonnet", Name: "Claude 3 Sonnet"},
				},
			},
		},
	}

	m, ok := reg.GetModel("anthropic", "claude-3-opus")
	if !ok {
		t.Error("GetModel('anthropic', 'claude-3-opus') should return true")
	}
	if m.ID != "claude-3-opus" {
		t.Errorf("GetModel returned wrong model: %+v", m)
	}

	_, ok = reg.GetModel("unknown", "claude-3-opus")
	if ok {
		t.Error("GetModel with unknown provider should return false")
	}

	_, ok = reg.GetModel("anthropic", "unknown-model")
	if ok {
		t.Error("GetModel with unknown model should return false")
	}
}

func TestRegistry_ProviderOptions(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{ID: "anthropic", Name: "Anthropic"},
			{ID: "openai", Name: "OpenAI"},
		},
	}

	opts := reg.ProviderOptions()
	if len(opts) != 2 {
		t.Errorf("ProviderOptions() returned %d options, want 2", len(opts))
	}

	if opts[0].Key != "Anthropic" || opts[0].Value != "anthropic" {
		t.Errorf("First option wrong: Key=%s, Value=%s", opts[0].Key, opts[0].Value)
	}
}

func TestRegistry_ModelOptions(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{
				ID:   "anthropic",
				Name: "Anthropic",
				Models: []Model{
					{ID: "claude-3-opus", Name: "Claude 3 Opus"},
					{ID: "claude-3-sonnet", Name: "Claude 3 Sonnet"},
				},
			},
		},
	}

	opts := reg.ModelOptions("anthropic")
	if len(opts) != 2 {
		t.Errorf("ModelOptions('anthropic') returned %d options, want 2", len(opts))
	}

	opts = reg.ModelOptions("unknown")
	if opts != nil {
		t.Error("ModelOptions('unknown') should return nil")
	}
}
