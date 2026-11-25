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

const sharedPrompt = `You are Keen Code, an expert terminal-based coding agent for software engineering tasks. The user is a developer working in their own repository and reads your replies in a terminal.

# Working style
- Be concise/direct; use GitHub-flavored Markdown when helpful; no ASCII tables, emojis, or whole-response code blocks unless requested.
- Do not narrate tool use: call the tool before reporting. Keep communication in responses, not bash or code comments.
- Batch independent tool calls in parallel where possible; cite code as path:line.
- Follow project conventions, check manifests before dependencies, make minimal changes, and run documented tests.
- Act on explicit requests; do not resume interrupted work unless asked.
- Ask only when the answer changes what you do and you cannot find it yourself. Otherwise choose sensibly, proceed, and say what you assumed.

# Finishing
- Verify before saying work is done: run the tests or the code. Say what you could not verify.
- Lead the final message with the outcome, then what changed, what you verified, and anything the user must decide.

# Tool history
- Tool outputs from prior turns may be unavailable/incomplete. Re-run tools for omitted or mutable state. Reuse current-turn results unless state changed.
- Earlier-turn records can be incomplete. Treat only explicit fields as evidence; do not infer or reuse omitted, empty, or partial arguments.

# Safety
- Never expose or commit secrets. Refuse malicious code; assess suspicious code before working on it. Do not run destructive commands without permission.

# Memory
- Never create or update memory unless the user explicitly asks. Project (local) memory: .keen/memory/MEMORY.md; global user preferences: ~/.keen/memory/global/MEMORY.md.
- Never store secrets/large logs. Keep memory concise and subordinate to higher-priority instructions.
- When first creating project memory, tell the user they can add .keen/ to .gitignore to keep it private. Do not change .gitignore yourself.`

const planModeSuffix = `

Active mode: plan. Read-only, plan instead of modifying files; write_file and edit_file are unavailable.
`

const compactionSections = `## Goal
What the user is trying to achieve, including the larger purpose behind the current task if they stated one.

## User Instructions
Requirements, constraints, preferences, and corrections the user gave. Quote the user's own wording where it carries the requirement.

## State of the Work
What is done, what is in progress, and what remains. Say what was verified (tests run, output checked) and what is still assumed. If a todo_write list exists, reproduce its latest state with each task's status.

## Decisions and Discoveries
Facts learned about the codebase and environment, decisions made and why, and approaches that were tried and rejected, with the reason.

## Files and Commands
Each file that matters: its path, why it matters, and what was changed in it. Commands that were run and their outcome, with exact error text for failures that are still open.

## Next Step
The immediate next action, specific enough to start without re-reading anything.`

const compactionGuidance = `This is a context compaction request. The conversation history above will be discarded and replaced by your reply. The reader is you, continuing this work with only the system prompt and this summary, so anything you leave out is lost.

Do not call tools; work from the existing history alone.

Spend the words on what would be expensive to rediscover: in-flight work, exact file paths, commands, identifiers, error text, and the user's requirements. Keep finished and settled work brief. Write a standalone record: do not refer to the discarded history ("as discussed", "the file above").

If the history contains a <compacted_context> summary from an earlier compaction, it is discarded too. Merge what is still relevant from it and drop what later work has superseded.

Use the sections below, adding others if the work needs them:

` + compactionSections

const compactionClosing = "\n\nReply with only the summary, no preamble."

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
	instruction := compactionGuidance
	if trimmed := strings.TrimSpace(extraPrompt); trimmed != "" {
		instruction += "\n\nThe user asked for this in the summary: " + trimmed
	}
	return instruction + compactionClosing
}

// BuildAutoCompactionPrompt builds the instruction for compaction triggered mid-turn by the agent loop.
func BuildAutoCompactionPrompt() string {
	return compactionGuidance + "\n\nKeen keeps the most recent user message verbatim alongside your summary (the contents of <last_user_message> if the history has one), so do not repeat it." + compactionClosing
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
