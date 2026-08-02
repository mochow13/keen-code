package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/keen-code/internal/skills"
)

func TestBuild_ContainsIdentity(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, "Keen Code") {
		t.Error("expected output to contain 'Keen Code'")
	}
}

func TestBuild_SharedPromptIsByteBounded(t *testing.T) {
	if len(sharedPrompt) > 2*1024 {
		t.Fatalf("shared prompt is %d bytes; expected at most %d", len(sharedPrompt), 2*1024)
	}
}

func TestBuild_ContainsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, dir) {
		t.Errorf("expected output to contain working dir %q", dir)
	}
}

func TestBuild_AgentsMd_Found(t *testing.T) {
	dir := t.TempDir()
	content := "## My Project\nSome instructions here."
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, "# Project Instructions") {
		t.Error("expected project instructions section")
	}
	if !strings.Contains(result, "My Project") {
		t.Error("expected AGENTS.md content in output")
	}
}

func TestBuild_AgentsMd_WalkUp(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir")
	os.MkdirAll(child, 0755)
	os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte("parent instructions"), 0644)

	result := Build(child, "", "", ModeBuild)
	if !strings.Contains(result, "parent instructions") {
		t.Error("expected AGENTS.md from parent directory")
	}
}

func TestBuild_ClaudeMd_Fallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude instructions"), 0644)

	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, "claude instructions") {
		t.Error("expected CLAUDE.md content as fallback")
	}
}

func TestBuild_NoInstructionFile(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "", ModeBuild)
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions section when no file exists")
	}
}

func TestBuild_AgentsMd_Truncation(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 10*1024)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, "[truncated") {
		t.Error("expected truncation note for large AGENTS.md")
	}
}

func TestBuild_AgentsMd_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(""), 0644)

	result := Build(dir, "", "", ModeBuild)
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions for empty AGENTS.md")
	}
}

func TestBuild_IncludesSkillsCatalog(t *testing.T) {
	dir := t.TempDir()
	catalog := skills.Catalog([]skills.Skill{{Name: "demo", Description: "Demo skill", Location: "/tmp/demo/SKILL.md"}}, skills.Config{})

	result := Build(dir, catalog, "", ModeBuild)
	if !strings.Contains(result, "## Available Skills") {
		t.Fatal("expected skills catalog")
	}
	if !strings.Contains(result, "- demo: Demo skill") {
		t.Fatalf("expected demo skill in catalog, got %q", result)
	}
}

func TestBuild_PlanIncludesPlanInstructions(t *testing.T) {
	result := Build(t.TempDir(), "", "", ModePlan)
	for _, expected := range []string{"# Active mode: plan", "write_file and edit_file are unavailable", "/mode build or Shift+Tab"} {
		if !strings.Contains(result, expected) {
			t.Fatalf("expected %q in plan prompt, got %q", expected, result)
		}
	}
}

func TestBuild_BuildIncludesBuildInstructions(t *testing.T) {
	result := Build(t.TempDir(), "", "", ModeBuild)
	if !strings.Contains(result, "# Active mode: build") {
		t.Fatalf("expected build mode prompt, got %q", result)
	}
	if strings.Contains(result, "write_file and edit_file are not available") {
		t.Fatalf("did not expect plan restrictions in build prompt, got %q", result)
	}
}

func TestBuild_IncludesToolFollowThroughInstructions(t *testing.T) {
	result := Build(t.TempDir(), "", "", ModeBuild)
	for _, expected := range []string{
		"Do not narrate tool use",
		"call the tool before reporting",
		"Batch independent tool calls in parallel where possible",
		"Earlier-turn records can be incomplete",
		"Treat only explicit fields as evidence",
		"do not infer or reuse omitted, empty, or partial arguments",
		"Re-run tools for omitted or mutable state",
		"Reuse current-turn results unless state changed",
		"Refuse malicious code; assess suspicious code before working on it",
		"Never create or update memory unless the user explicitly asks",
	} {
		if !strings.Contains(result, expected) {
			t.Fatalf("expected %q in prompt, got %q", expected, result)
		}
	}
}

func TestBuild_ModeInstructionsAreAtEnd(t *testing.T) {
	dir := t.TempDir()
	catalog := skills.Catalog([]skills.Skill{{Name: "demo", Description: "Demo skill", Location: "/tmp/demo/SKILL.md"}}, skills.Config{})

	result := Build(dir, catalog, "", ModePlan)
	modeIndex := strings.Index(result, "# Active mode: plan")
	if modeIndex == -1 {
		t.Fatal("expected active mode section")
	}
	if strings.Contains(result[modeIndex:], "Working directory:") {
		t.Fatal("expected working directory before mode section")
	}
	if strings.Contains(result[modeIndex:], "## Available Skills") {
		t.Fatal("expected skills catalog before mode section")
	}
}

func TestBuild_ProjectMemoryIncluded(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".keen")
	os.MkdirAll(memDir, 0755)
	memPath := filepath.Join(memDir, "MEMORY.md")
	os.WriteFile(memPath, []byte("- run go test -race ./... after Go changes"), 0644)

	result := Build(dir, "", "", ModeBuild)
	if !strings.Contains(result, "go test -race") {
		t.Fatal("expected memory content in prompt")
	}
}

func TestBuild_NoMemorySectionWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "", ModeBuild)
	if strings.Contains(result, "run go test -race ./... after Go changes") {
		t.Fatal("expected no loaded memory content when no memory file exists")
	}
}
