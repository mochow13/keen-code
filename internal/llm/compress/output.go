package compress

import (
	"maps"
	"strings"

	"github.com/mochow13/keen-code/internal/tools"
)

func ForLLM(name string, output any) any {
	result, ok := output.(map[string]any)
	if !ok {
		return output
	}

	switch name {
	case tools.ReadFileToolName:
		compact := cloneMap(result)
		delete(compact, "bytes_read")
		delete(compact, "lines_read")
		return compact
	case tools.GlobToolName:
		return compactGlob(result)
	case tools.GrepToolName:
		return compactGrep(result)
	default:
		return output
	}
}

func compactGlob(result map[string]any) any {
	files, ok := result["files"].([]string)
	if !ok {
		return result
	}

	prefix, relative := commonPathPrefix(files)
	if prefix == "" {
		return result
	}
	compact := cloneMap(result)
	compact["common_prefix"] = prefix
	compact["files"] = relative
	return compact
}

func compactGrep(result map[string]any) any {
	if _, ok := result["files"]; ok {
		return compactGlob(result)
	}

	matches, ok := result["matches"].([]map[string]any)
	if !ok {
		return result
	}

	paths := make([]string, len(matches))
	for i, match := range matches {
		path, ok := match["file"].(string)
		if !ok {
			return result
		}
		paths[i] = path
	}
	prefix, relative := commonPathPrefix(paths)
	if prefix == "" {
		return result
	}

	grouped := make(map[string][]map[string]any)
	for i, match := range matches {
		entry := cloneMap(match)
		delete(entry, "file")
		grouped[relative[i]] = append(grouped[relative[i]], entry)
	}
	compact := cloneMap(result)
	compact["common_prefix"] = prefix
	compact["matches"] = grouped
	return compact
}

func commonPathPrefix(paths []string) (string, []string) {
	if len(paths) == 0 {
		return "", nil
	}

	prefix := paths[0]
	for _, path := range paths[1:] {
		for !strings.HasPrefix(path, prefix) {
			if prefix == "" {
				return "", nil
			}
			prefix = prefix[:len(prefix)-1]
		}
	}

	boundary := strings.LastIndexAny(prefix, "/\\")
	if boundary < 0 {
		return "", nil
	}
	prefix = prefix[:boundary+1]
	if prefix == "" {
		return "", nil
	}

	relative := make([]string, len(paths))
	for i, path := range paths {
		relative[i] = strings.TrimPrefix(path, prefix)
	}
	return prefix, relative
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	maps.Copy(clone, source)
	return clone
}
