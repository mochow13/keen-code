package compress

import (
	"reflect"
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

func TestForLLM_ReadFileStripsDisplayOnlyMetadata(t *testing.T) {
	original := map[string]any{
		"content":     "1:abc|package main",
		"bytes_read":  18,
		"lines_read":  1,
		"total_lines": 2,
		"truncated":   true,
	}

	got := ForLLM(tools.ReadFileToolName, original)
	want := map[string]any{
		"content":     "1:abc|package main",
		"total_lines": 2,
		"truncated":   true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ForLLM() = %#v, want %#v", got, want)
	}
	if original["bytes_read"] != 18 || original["lines_read"] != 1 {
		t.Fatalf("original was modified: %#v", original)
	}
}

func TestForLLM_CompactsGlobFiles(t *testing.T) {
	got := ForLLM(tools.GlobToolName, map[string]any{
		"files": []string{"internal/llm/openai.go", "internal/llm/genkit.go"},
	})
	want := map[string]any{
		"common_prefix": "internal/llm/",
		"files":         []string{"openai.go", "genkit.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ForLLM() = %#v, want %#v", got, want)
	}
}

func TestForLLM_CompactsGrepMatchesByFile(t *testing.T) {
	got := ForLLM(tools.GrepToolName, map[string]any{
		"matches": []map[string]any{
			{"file": "internal/llm/openai.go", "line_number": 10, "line": "first", "line_hash": "aaa"},
			{"file": "internal/llm/openai.go", "line_number": 20, "line": "second", "line_hash": "bbb"},
			{"file": "internal/llm/genkit.go", "line_number": 30, "line": "third", "line_hash": "ccc"},
		},
	})
	want := map[string]any{
		"common_prefix": "internal/llm/",
		"matches": map[string][]map[string]any{
			"openai.go": {
				{"line_number": 10, "line": "first", "line_hash": "aaa"},
				{"line_number": 20, "line": "second", "line_hash": "bbb"},
			},
			"genkit.go": {
				{"line_number": 30, "line": "third", "line_hash": "ccc"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ForLLM() = %#v, want %#v", got, want)
	}
}

func TestForLLM_LeavesPathsWithoutSharedDirectoryUnchanged(t *testing.T) {
	original := map[string]any{"files": []string{"one.go", "two.go"}}
	if got := ForLLM(tools.GlobToolName, original); !reflect.DeepEqual(got, original) {
		t.Fatalf("ForLLM() = %#v, want %#v", got, original)
	}
}
