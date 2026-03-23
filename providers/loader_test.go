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

	for _, p := range reg.Providers {
		for _, m := range p.Models {
			if m.ContextWindow <= 0 {
				t.Errorf("model %s/%s has invalid context_window %d", p.ID, m.ID, m.ContextWindow)
			}
		}
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

func TestRegistry_GetModelContextWindow(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{
				ID: "openai",
				Models: []Model{
					{ID: "gpt-5.4", ContextWindow: 1050000},
				},
			},
		},
	}

	got, ok := reg.GetModelContextWindow("openai", "gpt-5.4")
	if !ok {
		t.Fatal("expected lookup success")
	}
	if got != 1050000 {
		t.Fatalf("expected 1050000, got %d", got)
	}

	if _, ok := reg.GetModelContextWindow("openai", "unknown"); ok {
		t.Fatal("expected unknown model lookup to fail")
	}

	if _, ok := reg.GetModelContextWindow("unknown", "gpt-5.4"); ok {
		t.Fatal("expected unknown provider lookup to fail")
	}
}
