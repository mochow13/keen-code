package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mochow13/keen-code/internal/memory"
)

type AgentMode string

const (
	ModeBuild AgentMode = "build"
	ModePlan  AgentMode = "plan"
	ModeYolo  AgentMode = "yolo"
)

const sharedPrompt = `You are Keen Code, an expert terminal-based coding agent for software engineering tasks.

# Working style
- Be concise/direct; use GitHub-flavored Markdown when helpful; no ASCII tables, emojis, or whole-response code blocks unless requested.
- Do not narrate tool use: call the tool before reporting. Keep communication in responses, not bash or code comments.
- Batch independent tool calls in parallel where possible; prefer dedicated file tools over bash; cite code as path:line.
- Follow project conventions, check manifests before dependencies, make minimal changes, and run documented tests.
- Act on explicit requests; do not resume interrupted work unless asked.
- Use dedicated filesystem tools; bash only for operations they do not cover.
- Track 3+ step work with todo_write: one task in_progress; mark each completed as soon as done.

# Output formats
- read_file provides N:HASH| anchors; use LINE:HASH with edit_file; re-read before later same-file edits.
- edit_file applies an atomic ops array to one file; all anchors must be from one snapshot.

# Tool history
- Tool outputs from prior turns may be unavailable/incomplete. Re-run tools for omitted or mutable state. Reuse current-turn results unless state changed.
- Earlier-turn records can be incomplete. Treat only explicit fields as evidence; do not infer or reuse omitted, empty, or partial arguments.

# Safety
- Never expose or commit secrets. Refuse malicious code; assess suspicious code before working on it. Do not run destructive commands without permission.

# Memory
- Never create or update memory unless the user explicitly asks. Project (local) memory: .keen/memory/MEMORY.md; global user preferences: ~/.keen/memory/global/MEMORY.md.
- Never store secrets/large logs. Keep memory concise and subordinate to higher-priority instructions.
- When first creating project memory, say: "Created .keen/memory/MEMORY.md. Add .keen/ to .gitignore if you want it private." Do not change .gitignore yourself.`

const planModeSuffix = `

Active mode: plan. Read-only, plan instead of modifying files; write_file and edit_file are unavailable.
`

const compactionSections = `## Goal
User objectives.

## Key Instructions
Important user constraints.

## Discoveries
Relevant codebase facts and requirements.

## Accomplished
Completed and remaining work, active progress, and next action. If a todo_write task list exists, reproduce its latest state with each task's status.

## Relevant Files
Relevant files, commands, errors, and tool results.`

const compactionGuidance = `This is a context compaction request: the earlier history will be discarded and replaced by your reply, which becomes the only record carried forward.

Never use any tools for this compaction request; work from the existing conversation history alone.

Summarize concisely but completely so work can continue without the earlier history. Keep exact file paths, commands, identifiers, and error text, and do not reference the discarded history (no "as discussed" or "the file above").

Cover at least the sections below, adding extra sections or detail when the work needs it:

` + compactionSections

const compactionPrompt = `Compact this conversation. ` + compactionGuidance

const maxInstructionsSize = 8 * 1024

func ModeUserSuffix(mode AgentMode) string {
	if mode == ModePlan {
		return planModeSuffix
	}
	return ""
}

func StripModeSuffix(content string) string {
	if stripped, ok := strings.CutSuffix(content, planModeSuffix); ok {
		return stripped
	}
	return content
}

func Build(workingDir, skillsCatalog, subagentsCatalog string) string {
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

	return sb.String()
}

// BuildCompactionPrompt builds the manual compaction instruction sent as the final user message.
func BuildCompactionPrompt(extraPrompt string) string {
	instruction := `Please compact this conversation. ` + compactionGuidance + `

Respond with only the structured summary, with no preamble.`
	if trimmed := strings.TrimSpace(extraPrompt); trimmed != "" {
		instruction += "\n\nIMPORTANT: Take the following instruction into consideration: " + trimmed
	}
	return instruction
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
