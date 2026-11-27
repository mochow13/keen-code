package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/skills"
)

func TestBuild_ContainsIdentity(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "")
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
	result := Build(dir, "", "")
	if !strings.Contains(result, dir) {
		t.Errorf("expected output to contain working dir %q", dir)
	}
}

func TestBuild_AgentsMd_Found(t *testing.T) {
	dir := t.TempDir()
	content := "## My Project\nSome instructions here."
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir, "", "")
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

	result := Build(child, "", "")
	if !strings.Contains(result, "parent instructions") {
		t.Error("expected AGENTS.md from parent directory")
	}
}

func TestBuild_ClaudeMd_Fallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude instructions"), 0644)

	result := Build(dir, "", "")
	if !strings.Contains(result, "claude instructions") {
		t.Error("expected CLAUDE.md content as fallback")
	}
}

func TestBuild_NoInstructionFile(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "")
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions section when no file exists")
	}
}

func TestBuild_AgentsMd_Truncation(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 10*1024)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir, "", "")
	if !strings.Contains(result, "[truncated") {
		t.Error("expected truncation note for large AGENTS.md")
	}
}

func TestBuild_AgentsMd_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(""), 0644)

	result := Build(dir, "", "")
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions for empty AGENTS.md")
	}
}

func TestBuild_IncludesSkillsCatalog(t *testing.T) {
	dir := t.TempDir()
	catalog := skills.Catalog([]skills.Skill{{Name: "demo", Description: "Demo skill", Location: "/tmp/demo/SKILL.md"}}, skills.Config{})

	result := Build(dir, catalog, "")
	if !strings.Contains(result, "## Available Skills") {
		t.Fatal("expected skills catalog")
	}
	if !strings.Contains(result, "- demo: Demo skill") {
		t.Fatalf("expected demo skill in catalog, got %q", result)
	}
}

func TestBuild_IsStableAcrossModes(t *testing.T) {
	dir := t.TempDir()
	catalog := skills.Catalog([]skills.Skill{{Name: "demo", Description: "Demo skill", Location: "/tmp/demo/SKILL.md"}}, skills.Config{})

	result := Build(dir, catalog, "")
	for _, unexpected := range []string{"Active mode:"} {
		if strings.Contains(result, unexpected) {
			t.Fatalf("expected %q to be absent from stable system prompt, got %q", unexpected, result)
		}
	}
}

func TestModeUserSuffix_PlanIncludesPlanInstructions(t *testing.T) {
	result := ModeUserSuffix(ModePlan)
	for _, expected := range []string{"Active mode: plan", "write_file, edit_file, and memory_write are unavailable"} {
		if !strings.Contains(result, expected) {
			t.Fatalf("expected %q in plan suffix, got %q", expected, result)
		}
	}
}

func TestModeUserSuffix_BuildHasNoSuffix(t *testing.T) {
	if got := ModeUserSuffix(ModeBuild); got != "" {
		t.Fatalf("expected no build mode suffix, got %q", got)
	}
}

func TestModeUserSuffix_YoloHasNoSuffix(t *testing.T) {
	if got := ModeUserSuffix(ModeYolo); got != "" {
		t.Fatalf("expected no yolo mode suffix, got %q", got)
	}
}

func TestStripModeSuffix_RemovesAppendedSuffix(t *testing.T) {
	if got := StripModeSuffix("hello" + ModeUserSuffix(ModePlan)); got != "hello" {
		t.Fatalf("expected plan suffix stripped, got %q", got)
	}
	if got := StripModeSuffix("hello"); got != "hello" {
		t.Fatalf("expected untouched content, got %q", got)
	}
}

func TestBuild_IncludesToolFollowThroughInstructions(t *testing.T) {
	result := Build(t.TempDir(), "", "")
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
		"Manage persistent memory with the memory_write tool on your own judgment",
	} {
		if !strings.Contains(result, expected) {
			t.Fatalf("expected %q in prompt, got %q", expected, result)
		}
	}
}

func TestBuild_ProjectMemoryIncluded(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, ".keen", "memory")
	os.MkdirAll(memDir, 0755)
	memPath := filepath.Join(memDir, "MEMORY.md")
	os.WriteFile(memPath, []byte("- run go test -race ./... after Go changes"), 0644)

	result := Build(dir, "", "")
	if !strings.Contains(result, "go test -race") {
		t.Fatal("expected memory content in prompt")
	}
}

func TestBuild_NoMemorySectionWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir, "", "")
	if strings.Contains(result, "run go test -race ./... after Go changes") {
		t.Fatal("expected no loaded memory content when no memory file exists")
	}
}

func TestBuildAuxiliaryPromptsIncludeWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, prompt := range map[string]string{
		"btw":       BuildBtwPrompt(dir),
		"adversary": BuildAdversaryPrompt(dir),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(prompt, dir) {
				t.Fatalf("prompt does not contain working directory %q", dir)
			}
		})
	}
}

func TestProjectInstructionsReadsNearestInstructions(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte("parent instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ProjectInstructions(child); !strings.Contains(got, "parent instructions") {
		t.Fatalf("ProjectInstructions() = %q", got)
	}
}

func TestBuildCompactionPromptTrimsExtraInstruction(t *testing.T) {
	got := BuildCompactionPrompt("  preserve failures  ")
	if !strings.Contains(got, "The user asked for this in the summary: preserve failures\n") {
		t.Fatalf("BuildCompactionPrompt() = %q", got)
	}
	for _, expected := range []string{
		"context compaction request",
		"Do not call tools",
		"<compacted_context>",
		"adding others if the work needs them",
		"## Goal",
		"## Next Step",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in compaction prompt, got %q", expected, got)
		}
	}
	if !strings.HasSuffix(got, compactionClosing) {
		t.Fatalf("expected compaction prompt to end with the closing line, got %q", got)
	}
	if strings.Contains(got, "<last_user_message>") {
		t.Fatalf("manual compaction prompt mentions the retained user message: %q", got)
	}
}

func TestBuildCompactionPromptWithoutExtraInstruction(t *testing.T) {
	if got := BuildCompactionPrompt(" \n "); got != compactionGuidance+compactionClosing {
		t.Fatalf("BuildCompactionPrompt() = %q", got)
	}
}

func TestBuildAutoCompactionPromptSharesCompactionFraming(t *testing.T) {
	got := BuildAutoCompactionPrompt()
	if !strings.HasPrefix(got, compactionGuidance) || !strings.HasSuffix(got, compactionClosing) {
		t.Fatalf("automatic compaction prompt does not share the manual framing: %q", got)
	}
	if !strings.Contains(got, "<last_user_message>") || !strings.Contains(got, "do not repeat it") {
		t.Fatalf("expected retained user message note in automatic compaction prompt, got %q", got)
	}
}
