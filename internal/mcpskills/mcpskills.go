package mcpskills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"gopkg.in/yaml.v3"
)

const (
	managedBy    = "keen-mcp"
	metaFileName = ".keen-generated-mcp.json"
	maxToolRows  = 1000
)

type metadata struct {
	ManagedBy             string `json:"managed_by"`
	Server                string `json:"server"`
	Status                string `json:"status"`
	ToolCount             int    `json:"tool_count"`
	LastSuccessfulRefresh string `json:"last_successful_refresh"`
	LastError             string `json:"last_error"`
}

func SkillName(server string) string {
	return "mcp:" + server
}

func IsSkillName(name string) bool {
	return strings.HasPrefix(name, "mcp:")
}

func ServerName(skillName string) string {
	return strings.TrimPrefix(skillName, "mcp:")
}

func SkillDir(server string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("mcpskills: home dir: %w", err)
	}
	return filepath.Join(skillsRoot(home), SkillName(server)), nil
}

func Remove(server string) error {
	dir, err := SkillDir(server)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func skillsRoot(home string) string {
	return filepath.Join(home, ".keen", "skills")
}

func Generate(server, description string, tools []keenmcp.Tool) error {
	dir, err := SkillDir(server)
	if err != nil {
		return err
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mcpskills: create skills dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(parent, ".mcp-"+server+"-tmp-")
	if err != nil {
		return fmt.Errorf("mcpskills: create temp dir: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	schemasDir := filepath.Join(tmpDir, "schemas")
	if err := os.Mkdir(schemasDir, 0o755); err != nil {
		return fmt.Errorf("mcpskills: create schemas dir: %w", err)
	}

	for _, tool := range tools {
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{}
		}
		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			continue
		}
		schemaPath, err := schemaFilePath(schemasDir, tool.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
			return fmt.Errorf("mcpskills: create schema dir %s: %w", tool.Name, err)
		}
		if err := os.WriteFile(schemaPath, data, 0o644); err != nil {
			return fmt.Errorf("mcpskills: write schema %s: %w", tool.Name, err)
		}
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "SKILL.md"), []byte(buildSkillMD(server, description, tools)), 0o644); err != nil {
		return fmt.Errorf("mcpskills: write SKILL.md: %w", err)
	}

	meta := metadata{
		ManagedBy:             managedBy,
		Server:                server,
		Status:                "connected",
		ToolCount:             len(tools),
		LastSuccessfulRefresh: time.Now().UTC().Format(time.RFC3339),
		LastError:             "",
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("mcpskills: marshal metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, metaFileName), metaData, 0o644); err != nil {
		return fmt.Errorf("mcpskills: write metadata: %w", err)
	}

	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("mcpskills: remove existing dir: %w", err)
	}
	if err := os.Rename(tmpDir, dir); err != nil {
		return fmt.Errorf("mcpskills: rename to target: %w", err)
	}
	committed = true
	return nil
}

func schemaFilePath(schemasDir, toolName string) (string, error) {
	clean := filepath.Clean(toolName)
	if clean == "." || clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", fmt.Errorf("mcpskills: invalid tool name %q", toolName)
	}
	return filepath.Join(schemasDir, clean+".json"), nil
}

func skillDescription(server, description string) string {
	description = strings.TrimSpace(strings.ReplaceAll(description, "\n", " "))
	if description != "" {
		return description
	}
	return "Use this skill to interact with the `" + server + "` MCP server."
}

func buildSkillMD(server, description string, tools []keenmcp.Tool) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	frontmatterData, err := yaml.Marshal(map[string]string{
		"name":        SkillName(server),
		"description": skillDescription(server, description),
	})
	if err == nil {
		sb.Write(frontmatterData)
	}
	sb.WriteString("---\n")
	sb.WriteString("## Calling tools\n")
	sb.WriteString("Before calling `call_mcp_tool`, always read the selected tool's schema file: `schemas/<tool_name>.json`.\n")
	sb.WriteString("Strictly follow the schema to pass the correct arguments to the tool.\n\n")
	sb.WriteString("Use the **exact** tool name from the \"Available tools\" table below. Do not guess,\n")
	sb.WriteString("abbreviate, or transform names (for example, do not swap `-` for `_` or do not change case).\n")
	sb.WriteString("If `call_mcp_tool` returns \"tool not found\", re-read this file and use the exact name of the correct tool from the table.\n\n")
	sb.WriteString("## Available tools\n")
	sb.WriteString("| Tool name | Description |\n")
	sb.WriteString("|------|-------------|\n")

	limit := min(len(tools), maxToolRows)
	for _, tool := range tools[:limit] {
		desc := strings.ReplaceAll(tool.Description, "\n", " ")
		desc = strings.ReplaceAll(desc, "|", "\\|")
		if len([]rune(desc)) > maxToolRows {
			desc = string([]rune(desc)[:maxToolRows]) + "..."
		}
		sb.WriteString("| " + tool.Name + " | " + desc + " |\n")
	}
	if len(tools) > maxToolRows {
		sb.WriteString(fmt.Sprintf("\n_%d more tools available (see schemas/)_\n", len(tools)-maxToolRows))
	}
	return sb.String()
}
