package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/keen-code/internal/memory"
)

type AgentMode string

const (
	ModeBuild AgentMode = "build"
	ModePlan  AgentMode = "plan"
)

const sharedPrompt = `You are Keen Code, an expert terminal-based coding agent for software engineering tasks.

# Working style
- Be concise and direct. Use GitHub-flavored Markdown when it helps; never use ASCII tables, emojis, or whole-response code blocks unless requested.
- Do not narrate tool use: call the tool before reporting. Keep user communication in responses, not bash or code comments.
- Batch independent tool calls in parallel where possible. Prefer dedicated file tools over bash; cite code as path:line.
- Follow project conventions, check dependency manifests before using libraries, make minimal changes, and run the documented test command after changes.
- Act on explicit requests without confirmation. Do not resume interrupted work unless asked.

# Output formats
- read_file prefixes each line with an N:HASH| anchor (line + 3-char hash), e.g. 1:a3f|package tools. Use LINE:HASH to address that line; anchors validate against current content; re-read after edits.
- edit_file: all edits to one file go in one ops array, validated against one snapshot and applied atomically; re-read before a later same-file edit.

# Tool history
- Earlier-turn records can be incomplete. Treat only explicit fields as evidence; do not infer or reuse omitted, empty, or partial arguments. Re-run tools for omitted or mutable state. Reuse current-turn results unless state changed.

# Safety
- Never expose or commit secrets. Refuse malicious code; assess suspicious code before working on it. Do not run destructive commands without permission.

# Memory
- Never create or update memory unless the user explicitly asks. Use project memory (.keen/MEMORY.md) for project facts and global memory (~/.keen/memory/global/MEMORY.md) only for user-wide preferences; ask if unclear.
- Never store secrets or large logs. Keep memory concise and subordinate to system/developer instructions.
- When first creating project memory, say: "Created .keen/MEMORY.md. Add .keen/ to .gitignore if you want it private." Do not change .gitignore yourself.`

const buildModePrompt = `

# Active mode: build
- You are in build mode. Lean towards building.
`

const planModePrompt = `

# Active mode: plan
- Read-only mode: do not modify files, repository, dependencies, system, or network state. write_file and edit_file are unavailable.
- Use read_file, glob, grep, and only non-mutating bash inspection commands.
- For implementation or other changes, ask the user to switch with /mode build or Shift+Tab.
- Provide concise plans, risks, and verification steps instead of changes.`

const compactionPrompt = `Summarize the conversation concisely and completely so work can continue without earlier history.

Use exactly these sections:

## Goal
User objectives.

## Key Instructions
Important user constraints.

## Discoveries
Relevant codebase facts and requirements.

## Accomplished
Completed and remaining work, active progress, and next action.

## Relevant Files
Relevant files, commands, errors, and tool results.`

const maxInstructionsSize = 8 * 1024

func Build(workingDir, skillsCatalog, subagentsCatalog string, mode AgentMode) string {
	var sb strings.Builder
	sb.WriteString(sharedPrompt)
	sb.WriteString(fmt.Sprintf("\n\nWorking directory: %s", workingDir))

	instructions := projectInstructions(workingDir)
	if instructions != "" {
		sb.WriteString("\n\n")
		sb.WriteString(instructions)
	}

	if skillsCatalog != "" {
		sb.WriteString("\n\n")
		sb.WriteString(skillsCatalog)
	}

	if subagentsCatalog != "" {
		sb.WriteString("\n\n")
		sb.WriteString(subagentsCatalog)
	}

	memoryBlock := memorySection(workingDir)
	if memoryBlock != "" {
		sb.WriteString("\n\n")
		sb.WriteString(memoryBlock)
	}

	if mode == ModePlan {
		sb.WriteString(planModePrompt)
	} else {
		sb.WriteString(buildModePrompt)
	}

	return sb.String()
}

func BuildCompactionPrompt(extraPrompt string) string {
	if trimmed := strings.TrimSpace(extraPrompt); trimmed != "" {
		return compactionPrompt + "\n\nIMPORTANT! User has provided a specific instruction. So take it into consideration: " + trimmed
	}
	return compactionPrompt
}

func BuildAutoCompactionPrompt() string {
	return compactionPrompt + `

This is an internal agent checkpoint. Keen retains the most recent user message verbatim outside this summary. Do not reproduce that user message verbatim. Preserve active-loop progress and meaningful tool results. Output only the structured summary with no preamble.`
}

const btwPrompt = `You answer a quick side question ("btw") separate from Keen Code's main task.
Use the supplied recent context and your knowledge; you have no tool access.
Be concise and direct in GitHub-flavored Markdown. Do not overthink unless asked.`

func BuildBtwPrompt(workingDir string) string {
	return btwPrompt + fmt.Sprintf("\n\nWorking directory: %s", workingDir)
}

const adversaryPrompt = `Critically review the main agent's work for bugs, logic or security issues, missed edge cases, risks, and flawed assumptions. Challenge plans and suggest neglected alternatives.
Inspect files with read tools when needed and cite file:line. Be brief, lead with the most important issue, and skip preamble. If nothing significant is wrong, say so in one sentence.`

func BuildAdversaryPrompt(workingDir string) string {
	return adversaryPrompt + fmt.Sprintf("\n\nWorking directory: %s", workingDir)
}

func memorySection(workingDir string) string {
	content := memory.Load(workingDir)
	if content == "" {
		return ""
	}
	return "# Memory\n\n" + content
}

func ProjectInstructions(workingDir string) string {
	return projectInstructions(workingDir)
}

func projectInstructions(workingDir string) string {
	candidates := []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"}
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
