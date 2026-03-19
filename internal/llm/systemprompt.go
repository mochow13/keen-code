package llm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const staticPrompt = `You are Keen Code, an expert coding agent running in terminal environment.

You help with software engineering tasks: fixing bugs, writing new features,
refactoring code, explaining code, exploring codebases, writing tests, and more.

# Tone and style
- Be concise and direct. Output is displayed on a CLI in a monospace font.
  Use GitHub-flavored markdown.
- No emojis unless the user explicitly asks for them.
- No unnecessary preamble or postamble. Do not summarise what you just did.
  Do not explain a code block you are about to write.
- One-word or one-line answers are fine when that is all the question needs.
- Never use bash or code comments as a communication channel — write to the
  user in your response text only.

# Doing tasks
- Explore before acting. Use grep/glob/read_file to understand the codebase
  before making changes.
- Follow existing conventions: mimic the style, naming, and patterns already
  in the project.
- Never assume a library is available. Check go.mod, package.json, pom.xml, or the
  relevant manifest before writing code that uses a dependency.
- Make minimal changes. Prefer editing an existing file to creating a new one.
- Verify your work. After making changes, run the project's test command if
  you know it. If you do not know it, check AGENTS.md, the README.md, or ask.

# Tool usage
- Prefer specialised tools over bash for file operations:
    read_file  → reading file contents
    write_file → creating new files
    edit_file  → modifying existing files
    glob       → listing files by pattern
    grep       → searching file contents
    bash       → shell commands that have no dedicated tool
- Run independent tool calls in parallel where possible.
- Reference code as file_path:line_number so the user can jump straight
  to the source.

# Git rules
- Never run git commit, git push, git reset, or git rebase unless the user
  explicitly asks you to.

# Safety
- Never introduce code that logs, exposes, or commits secrets or API keys.
- Refuse requests to write malicious code, even framed as educational.
- Before working on a file, consider what the code is supposed to do. If it
  looks malicious, refuse.`

const maxDirEntries = 40
const maxInstructionsSize = 8 * 1024

func Build(workingDir string) string {
	var sb strings.Builder
	sb.WriteString(staticPrompt)

	env := envBlock(workingDir)
	if env != "" {
		sb.WriteString("\n\n")
		sb.WriteString(env)
	}

	instructions := projectInstructions(workingDir)
	if instructions != "" {
		sb.WriteString("\n\n")
		sb.WriteString(instructions)
	}

	return sb.String()
}

func envBlock(workingDir string) string {
	gitRepo := "no"
	if isGitRepo(workingDir) {
		gitRepo = "yes"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<env>\n")
	fmt.Fprintf(&sb, "  Working directory: %s\n", workingDir)
	fmt.Fprintf(&sb, "  Platform: %s\n", runtime.GOOS)
	fmt.Fprintf(&sb, "  Today's date: %s\n", time.Now().Format("2006-01-02"))
	fmt.Fprintf(&sb, "  Is git repo: %s\n", gitRepo)
	fmt.Fprintf(&sb, "</env>")

	listing := dirListing(workingDir)
	if listing != "" {
		sb.WriteString("\n\nTop-level project structure:\n")
		sb.WriteString(listing)
	}

	return sb.String()
}

func dirListing(workingDir string) string {
	entries, err := os.ReadDir(workingDir)
	if err != nil {
		return ""
	}
	if len(entries) == 0 {
		return ""
	}

	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir
		}
		return entries[i].Name() < entries[j].Name()
	})

	var lines []string
	for i, entry := range entries {
		if i >= maxDirEntries {
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}

	return strings.Join(lines, "\n")
}

func projectInstructions(workingDir string) string {
	candidates := []string{"AGENTS.md", "CLAUDE.md"}
	path, content := findUpward(workingDir, candidates)
	if content == "" {
		return ""
	}

	if len(content) > maxInstructionsSize {
		content = content[:maxInstructionsSize] + fmt.Sprintf("\n[truncated — full file at %s]", path)
	}

	return fmt.Sprintf("# Project Instructions (from %s)\n\n%s", path, content)
}

func findUpward(dir string, candidates []string) (string, string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}

	for {
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err == nil {
				content := strings.TrimSpace(string(data))
				if content != "" {
					return path, content
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", ""
}

func isGitRepo(workingDir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
