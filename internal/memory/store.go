package memory

import (
	"fmt"
	"strings"
)

func CapFor(scope Scope) int {
	if scope == ScopeGlobal {
		return GlobalMaxBytes
	}
	return ProjectMaxBytes
}

func FormatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	kb := float64(n) / 1024
	if kb == float64(int(kb)) {
		return fmt.Sprintf("%d KB", int(kb))
	}
	return fmt.Sprintf("%.1f KB", kb)
}

// Add appends entry under the "## section" heading, creating the section at the
// end of the file if missing. An empty section appends to the end of the file.
func Add(content, section, entry string) (string, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", fmt.Errorf("entry must not be empty")
	}
	content = strings.TrimSpace(content)
	if content != "" && strings.Contains("\n"+content+"\n", "\n"+entry+"\n") {
		return "", fmt.Errorf("entry already exists in memory")
	}
	section = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(section), "#"))

	if section == "" {
		return joinBlocks(content, entry), nil
	}

	lines := splitLines(content)
	start, end := findSection(lines, section)
	if start < 0 {
		return joinBlocks(content, "## "+section+"\n\n"+entry), nil
	}

	insertAt := end
	for insertAt > start+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}
	insert := strings.Split(entry, "\n")
	if insertAt == start+1 {
		insert = append([]string{""}, insert...)
	}
	out := make([]string, 0, len(lines)+len(insert)+1)
	out = append(out, lines[:insertAt]...)
	out = append(out, insert...)
	if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) != "" {
		out = append(out, "")
	}
	out = append(out, lines[insertAt:]...)
	return finalize(strings.Join(out, "\n")), nil
}

// Replace swaps oldText for newText. oldText must occur exactly once. An empty
// newText removes the entry along with the blank lines it leaves behind.
func Replace(content, oldText, newText string) (string, error) {
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return "", fmt.Errorf("old_content must not be empty")
	}
	switch n := strings.Count(content, oldText); n {
	case 0:
		return "", fmt.Errorf("old_content not found in memory; copy the exact text of the entry from the memory section of the system prompt")
	case 1:
	default:
		return "", fmt.Errorf("old_content matches %d places in memory; include more surrounding text so it matches exactly one", n)
	}
	idx := strings.Index(content, oldText)
	before, after := content[:idx], content[idx+len(oldText):]
	newText = strings.TrimSpace(newText)
	if newText != "" {
		return finalize(before + newText + after), nil
	}
	before = strings.TrimRight(before, " \t")
	after = strings.TrimLeft(after, " \t")
	trailing := len(before) - len(strings.TrimRight(before, "\n"))
	leading := len(after) - len(strings.TrimLeft(after, "\n"))
	before = strings.TrimRight(before, "\n")
	after = strings.TrimLeft(after, "\n")
	if before == "" || after == "" {
		return finalize(before + after), nil
	}
	return finalize(before + strings.Repeat("\n", max(trailing, leading)) + after), nil
}

func findSection(lines []string, section string) (start, end int) {
	start = -1
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if start >= 0 {
			return start, i
		}
		if strings.HasPrefix(trimmed, "## ") && strings.EqualFold(strings.TrimSpace(trimmed[3:]), section) {
			start = i
		}
	}
	return start, len(lines)
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func joinBlocks(content, block string) string {
	if content == "" {
		return finalize(block)
	}
	return finalize(content + "\n\n" + block)
}

func finalize(content string) string {
	result := strings.TrimSpace(content)
	if result == "" {
		return ""
	}
	return result + "\n"
}
