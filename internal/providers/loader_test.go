package providers

import (
	"slices"
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

	providerIDs := make(map[string]bool, len(reg.Providers))
	for _, p := range reg.Providers {
		if providerIDs[p.ID] {
			t.Errorf("duplicate provider ID %q", p.ID)
		}
		providerIDs[p.ID] = true
		modelIDs := make(map[string]bool, len(p.Models))
		for _, m := range p.Models {
			if modelIDs[m.ID] {
				t.Errorf("duplicate model ID %q in provider %q", m.ID, p.ID)
			}
			modelIDs[m.ID] = true
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

func TestModel_ThinkingEffortsLoadFromYAML(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// claude-opus-5 should have thinking efforts
	m, ok := reg.GetModel("anthropic", "claude-opus-5")
	if !ok {
		t.Fatal("expected to find claude-opus-5")
	}
	if !m.SupportsThinkingEffort() {
		t.Error("expected claude-opus-5 to support thinking effort")
	}
	if len(m.ThinkingEfforts) != 5 {
		t.Errorf("expected 5 efforts for claude-opus-5, got %d: %v", len(m.ThinkingEfforts), m.ThinkingEfforts)
	}
	for _, effort := range m.ThinkingEfforts {
		if effort == "off" {
			t.Errorf("did not expect anthropic off effort, got %v", m.ThinkingEfforts)
		}
	}

	// claude-haiku-4-5 should NOT have thinking efforts
	haiku, ok := reg.GetModel("anthropic", "claude-haiku-4-5")
	if !ok {
		t.Fatal("expected to find claude-haiku-4-5")
	}
	if haiku.SupportsThinkingEffort() {
		t.Error("expected claude-haiku-4-5 to NOT support thinking effort")
	}

	// gpt-5.4 should have xhigh
	gpt, ok := reg.GetModel("openai", "gpt-5.4")
	if !ok {
		t.Fatal("expected to find gpt-5.4")
	}
	if !gpt.SupportsThinkingEffort() {
		t.Error("expected gpt-5.4 to support thinking effort")
	}
	foundXHigh := false
	for _, e := range gpt.ThinkingEfforts {
		if e == "xhigh" {
			foundXHigh = true
		}
	}
	if !foundXHigh {
		t.Errorf("expected gpt-5.4 to have xhigh effort, got %v", gpt.ThinkingEfforts)
	}

	codex, ok := reg.GetModel("openai-codex", "gpt-5.4")
	if !ok {
		t.Fatal("expected to find openai-codex/gpt-5.4")
	}
	if codex.ContextWindow != 272000 {
		t.Fatalf("expected openai-codex/gpt-5.4 context 272000, got %d", codex.ContextWindow)
	}

	deepseek, ok := reg.GetModel("deepseek", "deepseek-v4-pro")
	if !ok {
		t.Fatal("expected to find deepseek-v4-pro")
	}
	if !deepseek.SupportsThinkingEffort() {
		t.Error("expected deepseek-v4-pro to support thinking effort")
	}
	expectedDeepSeek := []string{"disabled", "high", "max"}
	if !slices.Equal(deepseek.ThinkingEfforts, expectedDeepSeek) {
		t.Fatalf("expected deepseek-v4-pro efforts %v, got %v", expectedDeepSeek, deepseek.ThinkingEfforts)
	}

	minimaxProvider, ok := reg.GetProvider("minimax")
	if !ok {
		t.Fatal("expected to find minimax provider")
	}
	if len(minimaxProvider.Models) != 3 {
		t.Fatalf("expected 3 minimax models, got %d", len(minimaxProvider.Models))
	}
	minimaxM27, ok := reg.GetModel("minimax", "MiniMax-M2.7")
	if !ok {
		t.Fatal("expected to find minimax/MiniMax-M2.7")
	}
	if minimaxM27.ContextWindow != 204800 {
		t.Fatalf("expected minimax-m2.7 context 204800, got %d", minimaxM27.ContextWindow)
	}
	if minimaxM27.SupportsThinkingEffort() {
		t.Fatalf("expected minimax-m2.7 to omit thinking efforts, got %v", minimaxM27.ThinkingEfforts)
	}

	opencode, ok := reg.GetProvider("opencode-go")
	if !ok {
		t.Fatal("expected to find opencode-go provider")
	}
	if len(opencode.Models) != 18 {
		t.Fatalf("expected 18 opencode-go models, got %d", len(opencode.Models))
	}

	qwen, ok := reg.GetModel("opencode-go", "qwen3.7-plus")
	if !ok {
		t.Fatal("expected to find opencode-go/qwen3.7-plus")
	}
	if qwen.ContextWindow != 1000000 {
		t.Fatalf("expected qwen3.7-plus context 1000000, got %d", qwen.ContextWindow)
	}
	expectedQwen := []string{"enabled", "disabled"}
	if !slices.Equal(qwen.ThinkingEfforts, expectedQwen) {
		t.Fatalf("expected qwen3.7-plus efforts %v, got %v", expectedQwen, qwen.ThinkingEfforts)
	}

	qwenMax, ok := reg.GetModel("opencode-go", "qwen3.8-max")
	if !ok {
		t.Fatal("expected to find opencode-go/qwen3.8-max")
	}
	if !qwenMax.SupportsThinkingEffort() {
		t.Fatal("expected qwen3.8-max to support thinking efforts")
	}

	minimax, ok := reg.GetModel("opencode-go", "minimax-m3")
	if !ok {
		t.Fatal("expected to find opencode-go/minimax-m3")
	}
	if !minimax.SupportsThinkingEffort() {
		t.Fatal("expected minimax-m3 to support thinking efforts")
	}
	expectedMiniMax := []string{"enabled", "adaptive", "disabled"}
	if !slices.Equal(minimax.ThinkingEfforts, expectedMiniMax) {
		t.Fatalf("expected minimax-m3 efforts %v, got %v", expectedMiniMax, minimax.ThinkingEfforts)
	}
}

func TestRegistry_GetModel(t *testing.T) {
	reg := &Registry{
		Providers: []Provider{
			{
				ID: "anthropic",
				Models: []Model{
					{ID: "claude-opus-4-6", ThinkingEfforts: []string{"low", "medium", "high", "max"}},
					{ID: "claude-haiku-4-5"},
				},
			},
		},
	}

	m, ok := reg.GetModel("anthropic", "claude-opus-4-6")
	if !ok {
		t.Fatal("expected to find claude-opus-4-6")
	}
	if !m.SupportsThinkingEffort() {
		t.Error("expected SupportsThinkingEffort() true")
	}

	haiku, ok := reg.GetModel("anthropic", "claude-haiku-4-5")
	if !ok {
		t.Fatal("expected to find claude-haiku-4-5")
	}
	if haiku.SupportsThinkingEffort() {
		t.Error("expected SupportsThinkingEffort() false for haiku")
	}

	_, ok = reg.GetModel("anthropic", "unknown")
	if ok {
		t.Error("expected GetModel to return false for unknown model")
	}

	_, ok = reg.GetModel("unknown", "claude-opus-4-6")
	if ok {
		t.Error("expected GetModel to return false for unknown provider")
	}
}

func TestResolveThinkingEffort(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		effort   string
		want     string
		wantErr  bool
	}{
		{name: "known value", provider: "minimax", model: "MiniMax-M3", effort: "adaptive", want: "adaptive"},
		{name: "unknown model", provider: "openai-compatible", model: "custom", effort: "custom", want: "custom"},
		{name: "unsupported value", provider: "deepseek", model: "deepseek-v4-pro", effort: "medium", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveThinkingEffort(tt.provider, tt.model, tt.effort)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveThinkingEffort() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ResolveThinkingEffort() = %q, want %q", got, tt.want)
			}
		})
	}
}
