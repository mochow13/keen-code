package mcpskills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keenmcp "github.com/mochow13/keen-code/internal/mcp"
)

func TestRemoveDeletesGeneratedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := Generate("github", "", []keenmcp.Tool{{Name: "search"}}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	dir := filepath.Join(home, ".keen", "skills", "mcp:github")

	if err := Remove("github"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected generated skill dir removed, err = %v", err)
	}
}

func TestGenerate_CreatesSkillDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tools := []keenmcp.Tool{
		{Name: "create_issue", Description: "Create a GitHub issue", InputSchema: map[string]any{"type": "object"}},
		{Name: "list_issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
	}

	if err := Generate("github", "Manage GitHub issues and pull requests.", tools); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	dir := filepath.Join(home, ".keen", "skills", "mcp:github")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("skill dir not created: %v", err)
	}

	skillMD, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}
	content := string(skillMD)
	if !strings.Contains(content, "name: mcp:github") {
		t.Errorf("SKILL.md missing name frontmatter")
	}
	if !strings.Contains(content, "description: Manage GitHub issues and pull requests.") {
		t.Errorf("SKILL.md missing server description")
	}
	if !strings.Contains(content, "create_issue") {
		t.Errorf("SKILL.md missing tool create_issue")
	}
	if !strings.Contains(content, "list_issues") {
		t.Errorf("SKILL.md missing tool list_issues")
	}

	for _, toolName := range []string{"create_issue", "list_issues"} {
		schemaPath := filepath.Join(dir, "schemas", toolName+".json")
		if _, err := os.Stat(schemaPath); err != nil {
			t.Errorf("schema file %s not created", schemaPath)
		}
	}

	metaData, err := os.ReadFile(filepath.Join(dir, metaFileName))
	if err != nil {
		t.Fatalf("metadata file not created: %v", err)
	}
	var meta metadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.ManagedBy != managedBy {
		t.Errorf("metadata managed_by = %q, want %q", meta.ManagedBy, managedBy)
	}
	if meta.Server != "github" {
		t.Errorf("metadata server = %q, want %q", meta.Server, "github")
	}
	if meta.Status != "connected" {
		t.Errorf("metadata status = %q, want %q", meta.Status, "connected")
	}
	if meta.ToolCount != 2 {
		t.Errorf("metadata tool_count = %d, want 2", meta.ToolCount)
	}
}

func TestGenerate_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tools := []keenmcp.Tool{{Name: "ping", Description: "Ping", InputSchema: nil}}

	if err := Generate("myserver", "", tools); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	if err := Generate("myserver", "", tools); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}

	dir := filepath.Join(home, ".keen", "skills", "mcp:myserver")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("skill dir missing after second Generate: %v", err)
	}
}

func TestGenerate_QuotesFrontmatter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Generate("git:hub", "", nil); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	skillMD, err := os.ReadFile(filepath.Join(home, ".keen", "skills", "mcp:git:hub", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(skillMD), "name: mcp:git:hub") {
		t.Fatalf("SKILL.md missing quoted-safe name: %s", string(skillMD))
	}
}

func TestGenerate_RejectsPathTraversalToolName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := Generate("srv", "", []keenmcp.Tool{{Name: "../escape", Description: "bad"}})
	if err == nil {
		t.Fatal("expected invalid tool name error")
	}
}

func TestGenerate_FallsBackToStaticDescription(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Generate("github", " \n\t ", nil); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	skillMD, err := os.ReadFile(filepath.Join(home, ".keen", "skills", "mcp:github", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(skillMD), "description: Use this skill to interact with the `github` MCP server.") {
		t.Fatalf("SKILL.md missing fallback description: %s", string(skillMD))
	}
}

func TestBuildSkillMD_CapsAt1000Tools(t *testing.T) {
	tools := make([]keenmcp.Tool, 1020)
	for i := range tools {
		tools[i] = keenmcp.Tool{Name: "tool", Description: "desc"}
	}
	md := buildSkillMD("srv", "", tools)
	if !strings.Contains(md, "20 more tools") {
		t.Errorf("expected overflow notice, got:\n%s", md)
	}
}

func TestBuildSkillMD_TruncatesLongDescription(t *testing.T) {
	long := strings.Repeat("a", 1200)
	tools := []keenmcp.Tool{{Name: "t", Description: long}}
	md := buildSkillMD("srv", "", tools)
	if strings.Contains(md, strings.Repeat("a", 1200)) {
		t.Errorf("expected description to be truncated")
	}
}
