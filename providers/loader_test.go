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
