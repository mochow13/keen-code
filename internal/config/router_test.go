package config

import "testing"

func TestRouterConfigCategoriesAndPersistence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var unset *RouterConfig
	if unset.Configured() {
		t.Fatal("nil router configured")
	}
	router := &RouterConfig{Enabled: true,
		Simple:   RouterModel{Provider: "openai", Model: "simple", ThinkingEffort: "low"},
		Standard: RouterModel{Provider: "anthropic", Model: "standard", ThinkingEffort: "medium"},
		Complex:  RouterModel{Provider: "openai-codex", Model: "complex", ThinkingEffort: "high"},
	}
	if !router.Configured() {
		t.Fatal("complete router not configured")
	}
	for category, expected := range map[string]RouterModel{RouterCategorySimple: router.Simple, RouterCategoryStandard: router.Standard, RouterCategoryComplex: router.Complex, "unknown": router.Standard, "": router.Standard} {
		if got := router.ModelForCategory(category); got != expected {
			t.Fatalf("category %q: %v, want %v", category, got, expected)
		}
	}
	global := DefaultGlobalConfig()
	global.Router = router
	loader := NewLoader()
	if err := loader.Save(global); err != nil {
		t.Fatal(err)
	}
	loaded, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Router == nil || *loaded.Router != *router {
		t.Fatal("router activation or selections not persisted")
	}
	router.Complex.Model = ""
	if router.Configured() {
		t.Fatal("incomplete router configured")
	}
}
