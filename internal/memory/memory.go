package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	GlobalMaxBytes  = 8 * 1024
	ProjectMaxBytes = 16 * 1024
	memoryFileName  = "MEMORY.md"
)

type Scope string

const (
	ScopeGlobal  Scope = "Global"
	ScopeProject Scope = "Project"
)

type File struct {
	Path    string
	Scope   Scope
	Exists  bool
	Content string
}

func GlobalPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".keen", "memory", "global", memoryFileName)
}

func ProjectPath(workingDir string) string {
	if workingDir == "" {
		return ""
	}
	return filepath.Join(workingDir, ".keen", "memory", memoryFileName)
}

func Files(workingDir string) (global, project File) {
	return FilesAt(GlobalPath(), ProjectPath(workingDir))
}

func FilesAt(globalPath, projectPath string) (global, project File) {
	global = File{Path: globalPath, Scope: ScopeGlobal}
	if globalPath != "" {
		global.Content = readMemory(globalPath)
		global.Exists = global.Content != ""
	}

	project = File{Path: projectPath, Scope: ScopeProject}
	if projectPath != "" {
		project.Content = readMemory(projectPath)
		project.Exists = project.Content != ""
	}
	return global, project
}

func readMemory(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cap(content string, max int) string {
	if len(content) <= max {
		return content
	}
	cut := strings.LastIndex(content[:max+1], "\n")
	if cut <= 0 {
		cut = max
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
	}
	return strings.TrimRight(content[:cut], "\n") + "\n[truncated — memory exceeds size cap; read the full file with read_file, then merge, trim, or remove entries with the memory_write tool]"
}

func heading(f File) string {
	max := CapFor(f.Scope)
	return fmt.Sprintf("# %s memory (%s, %s of %s)", f.Scope, f.Path, FormatSize(len(f.Content)), FormatSize(max))
}

func Load(workingDir string) string {
	return LoadAt(GlobalPath(), ProjectPath(workingDir))
}

func LoadAt(globalPath, projectPath string) string {
	global, project := FilesAt(globalPath, projectPath)
	var sb strings.Builder
	if global.Exists {
		sb.WriteString(fmt.Sprintf("%s\n\n%s", heading(global), cap(global.Content, GlobalMaxBytes)))
	}
	if project.Exists {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%s\n\n%s", heading(project), cap(project.Content, ProjectMaxBytes)))
	}
	return sb.String()
}
