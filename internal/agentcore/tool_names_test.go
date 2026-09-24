package agentcore

import (
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

// Guards the agentcore/tools tool-name boundary: agentcore.ToolName*
// are aliases of tools.*ToolName, so a rename on either side fails
// loudly here instead of silently breaking render hiding (stream_render.go),
// ask-user filtering, or retainedHistoricalToolInputs (turn_memory.go).
func TestToolNamesMatchToolsPackage(t *testing.T) {
	pairs := []struct {
		agentcoreName string
		toolsName     string
	}{
		{ToolNameAskUser, tools.AskUserToolName},
		{ToolNameBash, tools.BashToolName},
		{ToolNameCallMCP, tools.CallMCPToolName},
		{ToolNameDelegate, tools.DelegateToolName},
		{ToolNameEditFile, tools.EditFileToolName},
		{ToolNameGlob, tools.GlobToolName},
		{ToolNameGrep, tools.GrepToolName},
		{ToolNameReadFile, tools.ReadFileToolName},
		{ToolNameWebFetch, tools.WebFetchToolName},
		{ToolNameWriteFile, tools.WriteFileToolName},
	}
	if len(pairs) != 10 {
		t.Fatalf("expected 10 tool-name pairs, got %d; update when adding tools", len(pairs))
	}
	seen := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if p.agentcoreName == "" || p.toolsName == "" {
			t.Errorf("empty tool name in pair %+v", p)
		}
		if p.agentcoreName != p.toolsName {
			t.Errorf("agentcore name %q != tools name %q", p.agentcoreName, p.toolsName)
		}
		if prev, dup := seen[p.agentcoreName]; dup {
			t.Errorf("duplicate tool name %q (also %q)", p.agentcoreName, prev)
		}
		seen[p.agentcoreName] = p.toolsName
	}
}
