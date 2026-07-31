package subagents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverAndLoadMetadata(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := filepath.Join(workingDir, ".keen", "agents")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project agents dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
permissions:
  - read
---
Review with focus.
`), 0o644); err != nil {
		t.Fatalf("write project profile: %v", err)
	}

	discovery := Discover(workingDir)
	if len(discovery.Profiles) != 1 {
		t.Fatalf("expected 1 discovered profile, got %d: %+v", len(discovery.Profiles), discovery.Profiles)
	}

	loaded := LoadMetadata(discovery)
	if len(loaded.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", loaded.Warnings)
	}
	if len(loaded.Profiles) != 1 {
		t.Fatalf("expected 1 loaded profile, got %d: %+v", len(loaded.Profiles), loaded.Profiles)
	}
	if loaded.Profiles[0].Name != "reviewer" {
		t.Fatalf("expected reviewer profile, got %+v", loaded.Profiles)
	}
	if loaded.Profiles[0].Instructions != "Review with focus." {
		t.Fatalf("unexpected reviewer instructions: %q", loaded.Profiles[0].Instructions)
	}
}

func TestLoadMetadataWarnsAndSkipsUnreadableProfile(t *testing.T) {
	discovery := Discovery{
		Profiles:  []Profile{{Name: "missing", Description: "missing"}},
		locations: []string{filepath.Join(t.TempDir(), "missing.md")},
	}

	loaded := LoadMetadata(discovery)
	if len(loaded.Profiles) != 0 {
		t.Fatalf("expected missing profile to be skipped, got %+v", loaded.Profiles)
	}
	if len(loaded.Warnings) != 1 || !strings.Contains(loaded.Warnings[0], "failed to load") {
		t.Fatalf("expected failed load warning, got %v", loaded.Warnings)
	}
}

func TestCatalogSkipsHiddenProfiles(t *testing.T) {
	catalog := Catalog([]Profile{
		{Name: "visible", Description: "Visible work."},
		{Name: "hidden", Description: "Hidden work.", Hidden: true},
	})

	if !strings.Contains(catalog, "visible: Visible work.") {
		t.Fatalf("expected visible profile in catalog: %s", catalog)
	}
	if strings.Contains(catalog, "hidden") {
		t.Fatalf("expected hidden profile to be omitted: %s", catalog)
	}
}

func TestCatalogSupportsCapableWorkerProfiles(t *testing.T) {
	catalog := Catalog([]Profile{{
		Name:        "worker",
		Description: "Implements focused changes with write and bash capabilities.",
	}})
	for _, expected := range []string{
		"up to 10 bounded tasks",
		"Only the profile names listed below are valid delegate_task agents",
		"Skills are not subagents",
		"descriptions and configured capabilities",
		"relevant paths, inputs, constraints, and expected results",
		"one-shot run",
		"cannot ask the child follow-up questions or resume its context",
	} {
		if !strings.Contains(catalog, expected) {
			t.Fatalf("expected catalog guidance to contain %q, got %q", expected, catalog)
		}
	}
	for _, discouraged := range []string{"independent read-only work", "direct edits, commands"} {
		if strings.Contains(catalog, discouraged) {
			t.Fatalf("catalog retained obsolete worker restriction %q: %q", discouraged, catalog)
		}
	}
}

func TestCatalogEmptyWhenNoVisibleProfiles(t *testing.T) {
	if got := Catalog([]Profile{{Name: "hidden", Description: "Hidden work.", Hidden: true}}); got != "" {
		t.Fatalf("expected empty catalog, got %q", got)
	}
}
