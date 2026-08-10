package output

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/llm"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	maxDisplayPathRunes    = 40
	maxDisplayPatternRunes = 50
	maxDisplayValueRunes   = 40
	maxDisplayErrorRunes   = 80
	maxGenericInputFields  = 2
)

type OutputBuilder struct {
	lines      []string
	width      int
	workingDir string
}

func NewOutputBuilder(width int, workingDir string) *OutputBuilder {
	return &OutputBuilder{
		lines:      []string{},
		width:      width,
		workingDir: workingDir,
	}
}

func (ob *OutputBuilder) SetLines(lines []string) {
	ob.lines = lines
}

func (ob *OutputBuilder) GetLines() []string {
	return ob.lines
}

func (ob *OutputBuilder) AddLine(line string) {
	ob.lines = append(ob.lines, line)
}

func (ob *OutputBuilder) AddEmptyLine() {
	ob.lines = append(ob.lines, "")
}

func (ob *OutputBuilder) SetWidth(width int) {
	ob.width = width
}

func (ob *OutputBuilder) AddUserInput(input string, promptStyle lipgloss.Style) {
	inputLines := strings.Split(input, "\n")
	const (
		promptWidth            = 3
		blockHorizontalPadding = 2
	)
	blockContentWidth := ob.width - blockHorizontalPadding
	if blockContentWidth < 1 {
		blockContentWidth = 1
	}
	wrapWidth := blockContentWidth - promptWidth
	if wrapWidth < 1 {
		wrapWidth = 1
	}

	bg := repltheme.UserInputBlockBackground
	wrapStyle := lipgloss.NewStyle().Width(wrapWidth).Background(bg)
	indentStyle := lipgloss.NewStyle().Background(bg)
	prompt := promptStyle.UnsetMarginTop().Background(bg).Render(" ▶ ")

	bodyLines := make([]string, 0, len(inputLines))
	for i, inputLine := range inputLines {
		prefix := indentStyle.Render("   ")
		if i == 0 {
			prefix = prompt
		}

		wrappedLines := strings.Split(wrapStyle.Render(inputLine), "\n")
		for j, wrappedLine := range wrappedLines {
			if j > 0 {
				prefix = indentStyle.Render("   ")
			}
			bodyLines = append(bodyLines, prefix+wrappedLine)
		}
	}
	body := strings.Join(bodyLines, "\n")

	rendered := repltheme.UserInputBlockStyle.Width(ob.width).Render(body)
	for line := range strings.SplitSeq(rendered, "\n") {
		ob.lines = append(ob.lines, line)
	}
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddAssistantResponse(response string, assistantStyle lipgloss.Style) {
	wrapStyle := lipgloss.NewStyle().Width(ob.width - 4)
	responseLines := strings.SplitSeq(response, "\n")
	for line := range responseLines {
		ob.lines = append(ob.lines, "  "+wrapStyle.Render(assistantStyle.Render(line)))
	}
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddError(err string, errorStyle lipgloss.Style) {
	wrapStyle := lipgloss.NewStyle().Width(ob.width - 4)
	ob.lines = append(ob.lines, wrapStyle.Render(errorStyle.Render("  Error: "+err)))
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddStyledLine(content string, style lipgloss.Style) {
	ob.lines = append(ob.lines, style.Render(content))
}

func (ob *OutputBuilder) Join() string {
	if len(ob.lines) == 0 {
		return ""
	}
	return strings.Join(ob.lines, "\n")
}

func (ob *OutputBuilder) IsEmpty() bool {
	return len(ob.lines) == 0
}

func (ob *OutputBuilder) AddToolStart(toolCall *llm.ToolCall) {
	ob.lines = append(ob.lines, FormatToolStart(toolCall, ob.workingDir))
}

func (ob *OutputBuilder) AddToolEnd(toolCall *llm.ToolCall) {
	ob.lines = append(ob.lines, FormatToolEnd(toolCall))
}

var toolDisplayNames = map[string]string{
	"read_file":     "Read",
	"write_file":    "Write",
	"edit_file":     "Edit",
	"grep":          "Search",
	"glob":          "Find",
	"bash":          "Run",
	"web_fetch":     "Fetch",
	"call_mcp_tool": "MCP",
	"delegate_task": "Delegate",
}

var toolLabelCaser = cases.Title(language.English)

func toolDisplayName(name string) string {
	if label, ok := toolDisplayNames[name]; ok {
		return label
	}
	return toolLabelCaser.String(strings.ReplaceAll(name, "_", " "))
}

func FormatToolStart(toolCall *llm.ToolCall, workingDir string) string {
	detail := formatToolInputDetail(toolCall.Name, toolCall.Input, workingDir)
	return "  " + renderToolStatus("●", toolCall.Name, toolDisplayName(toolCall.Name), detail, nil, nil, false) + repltheme.ToolMetaStyle.Render("...")
}

func FormatToolDone(startCall, endCall *llm.ToolCall, workingDir string) string {
	detail := formatToolInputDetail(startCall.Name, startCall.Input, workingDir)
	metadata, failed := formatToolResultMetadata(startCall.Name, endCall)
	errText := formatToolError(startCall.Name, endCall.Error)
	if failed {
		return "  " + renderToolStatus("✗", startCall.Name, toolDisplayName(startCall.Name), detail, metadata, errText, true)
	}
	return "  " + renderToolStatus("✓", startCall.Name, toolDisplayName(startCall.Name), detail, metadata, errText, false)
}

func FormatSubagentTool(agent string, startCall, endCall *llm.ToolCall, workingDir string) string {
	prefix := "[" + agent + "] "
	if startCall == nil {
		if endCall == nil {
			return ""
		}
		cloned := *endCall
		cloned.Output = nil
		return strings.Replace(FormatToolEnd(&cloned), toolDisplayName(cloned.Name), prefix+toolDisplayName(cloned.Name), 1)
	}
	if endCall == nil {
		return strings.Replace(FormatToolStart(startCall, workingDir), toolDisplayName(startCall.Name), prefix+toolDisplayName(startCall.Name), 1)
	}
	cloned := *endCall
	cloned.Output = nil
	return strings.Replace(FormatToolDone(startCall, &cloned, workingDir), toolDisplayName(startCall.Name), prefix+toolDisplayName(startCall.Name), 1)
}

func FormatToolEnd(toolCall *llm.ToolCall) string {
	metadata, failed := formatToolResultMetadata(toolCall.Name, toolCall)
	errText := formatToolError(toolCall.Name, toolCall.Error)
	if failed {
		return "  " + renderToolStatus("✗", toolCall.Name, toolDisplayName(toolCall.Name), "", metadata, errText, true)
	}
	return "  " + renderToolStatus("✓", toolCall.Name, toolDisplayName(toolCall.Name), "", metadata, errText, false)
}

func formatToolError(toolName, message string) *string {
	if message == "" {
		return nil
	}
	if toolName == "call_mcp_tool" {
		minimal := "failed"
		return &minimal
	}
	compacted := compactDisplayValue(message, maxDisplayErrorRunes)
	return &compacted
}

func renderToolStatus(marker, toolName, label, detail string, metadata []string, errText *string, failed bool) string {
	var b strings.Builder
	b.WriteString(repltheme.ToolNameStyle.Render(marker + " " + label))
	if detail != "" {
		b.WriteString(" ")
		b.WriteString(repltheme.ToolMetaStyle.Render("→"))
		b.WriteString(" ")
		if failed && errText == nil {
			b.WriteString(repltheme.ToolErrorStyle.Render(detail))
		} else {
			b.WriteString(renderToolDetail(toolName, detail))
		}
	}
	for _, meta := range metadata {
		if meta == "" {
			continue
		}
		b.WriteString(" ")
		b.WriteString(repltheme.ToolMetaStyle.Render("· " + meta))
	}
	if errText != nil {
		b.WriteString(" ")
		b.WriteString(repltheme.ToolMetaStyle.Render("· "))
		b.WriteString(repltheme.ToolErrorStyle.Render(*errText))
	}
	return b.String()
}

func renderToolDetail(toolName, detail string) string {
	if toolName != "grep" && toolName != "glob" {
		return detail
	}

	before, after, found := strings.Cut(detail, " in ")
	if !found {
		return detail
	}
	return before + repltheme.ToolMetaStyle.Render(" in ") + after
}

func formatToolInputDetail(toolName string, input map[string]any, workingDir string) string {
	if input == nil {
		return ""
	}

	switch toolName {
	case "call_mcp_tool":
		return formatMCPToolInput(input)
	case "delegate_task":
		return formatDelegateTaskInput(input)
	case "read_file", "write_file", "edit_file":
		if path, ok := input["path"].(string); ok && path != "" {
			return compactDisplayPath(formatToolPathForUI(path, workingDir))
		}
		return ""
	case "grep", "glob":
		return formatSearchInput(input, workingDir)
	case "bash":
		if command, ok := input["command"].(string); ok && command != "" {
			return compactDisplayValue(command, maxDisplayValueRunes)
		}
		return ""
	case "web_fetch":
		if url, ok := input["url"].(string); ok && url != "" {
			return compactDisplayValue(url, maxDisplayValueRunes)
		}
		return ""
	}

	return formatGenericInput(input)
}

func formatSearchInput(input map[string]any, workingDir string) string {
	pattern, _ := input["pattern"].(string)
	if pattern != "" {
		pattern = compactDisplayValue(pattern, maxDisplayPatternRunes)
		pattern = formatSearchPatternForDisplay(pattern)
	}
	path, _ := input["path"].(string)
	if path != "" {
		path = compactDisplayPath(formatToolPathForUI(path, workingDir))
	}

	switch {
	case pattern != "" && path != "":
		return pattern + " in " + path
	case pattern != "":
		return pattern
	case path != "":
		return path
	default:
		return ""
	}
}

func formatSearchPatternForDisplay(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern))
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) && strings.ContainsRune(`\\.^$*+?()[]{}|`, rune(pattern[i+1])) {
			i++
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}

func formatGenericInput(input map[string]any) string {
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, maxGenericInputFields+1)
	for _, key := range keys {
		if len(parts) >= maxGenericInputFields {
			parts = append(parts, fmt.Sprintf("+%d", len(keys)-maxGenericInputFields))
			break
		}
		if rendered, ok := formatGenericValue(input[key]); ok {
			parts = append(parts, key+"="+rendered)
		}
	}
	return strings.Join(parts, " ")
}

func formatGenericValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return compactDisplayValue(v, maxDisplayValueRunes), true
	case int, int64, float64, bool:
		return fmt.Sprintf("%v", v), true
	case map[string]any:
		return "{…}", true
	case []any:
		return fmt.Sprintf("[%d]", len(v)), true
	case nil:
		return "", false
	default:
		return "{…}", true
	}
}

func formatToolResultMetadata(toolName string, endCall *llm.ToolCall) ([]string, bool) {
	if endCall.Error != "" {
		return withDuration(nil, endCall.Duration), true
	}

	var metadata []string
	if result, ok := endCall.Output.(map[string]any); ok {
		switch toolName {
		case "read_file":
			metadata = readFileMetadata(result)
		case "grep":
			metadata = grepMetadata(result)
		case "glob":
			if count, ok := intValue(result["count"]); ok {
				metadata = append(metadata, pluralize(count, "file"))
			}
		case "write_file":
			if created, ok := result["created"].(bool); ok {
				if created {
					metadata = append(metadata, "created")
				} else {
					metadata = append(metadata, "updated")
				}
			}
			if bytes, ok := intValue(result["bytes_written"]); ok {
				metadata = append(metadata, formatByteCount(bytes))
			}
		case "edit_file":
			if count, ok := intValue(result["replacementCount"]); ok {
				metadata = append(metadata, pluralize(count, "replacement"))
			}
		case "web_fetch":
			if content, ok := result["content"].(string); ok {
				metadata = append(metadata, formatByteCount(len(content)))
			}
		case "bash":
			if code, ok := intValue(result["exit_code"]); ok && code != 0 {
				metadata = append(metadata, fmt.Sprintf("exit %d", code))
			}
		case "delegate_task":
			summary, failed := delegateTaskSummary(result)
			if summary != "" {
				metadata = append([]string{summary}, metadata...)
				if failed {
					return withDuration(metadata, endCall.Duration), true
				}
			}
		case "call_mcp_tool":
			if truncated, _ := result["truncated"].(bool); truncated {
				metadata = append(metadata, "truncated")
			}
		}
	}

	return withDuration(metadata, endCall.Duration), false
}

func readFileMetadata(result map[string]any) []string {
	var metadata []string
	if lines, ok := intValue(result["total_lines"]); ok {
		metadata = append(metadata, pluralize(lines, "line"))
	}
	if bytes, ok := intValue(result["bytes_read"]); ok {
		metadata = append(metadata, formatByteCount(bytes))
	}
	return metadata
}

func grepMetadata(result map[string]any) []string {
	var metadata []string
	mode, _ := result["output_mode"].(string)
	count, ok := intValue(result["count"])
	if !ok {
		return metadata
	}
	if mode == "file" {
		metadata = append(metadata, pluralize(count, "file"))
	} else {
		metadata = append(metadata, pluralize(count, "match"))
	}
	return metadata
}

func delegateTaskSummary(result map[string]any) (string, bool) {
	completed, failed, completedByAgent, failedByAgent, ok := delegateResultCounts(result)
	if !ok {
		return "", false
	}

	status := fmt.Sprintf("%d completed", completed)
	if agents := formatAgentCounts(completedByAgent); agents != "" {
		status += " (" + agents + ")"
	}
	if failed > 0 {
		status += fmt.Sprintf(", %d failed", failed)
		if agents := formatAgentCounts(failedByAgent); agents != "" {
			status += " (" + agents + ")"
		}
		return status, true
	}
	return status, false
}

func withDuration(metadata []string, duration time.Duration) []string {
	if duration > 0 {
		metadata = append(metadata, formatToolDuration(duration))
	}
	return metadata
}

func formatToolDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return "0ms"
	}
	switch {
	case duration >= time.Minute:
		return fmt.Sprintf("%dm%ds", int(duration/time.Minute), int((duration%time.Minute)/time.Second))
	case duration >= 10*time.Second:
		return fmt.Sprintf("%ds", int(duration/time.Second))
	case duration >= time.Second:
		return fmt.Sprintf("%.1fs", float64(duration)/float64(time.Second))
	case duration >= time.Millisecond:
		return fmt.Sprintf("%dms", duration.Milliseconds())
	default:
		return "0ms"
	}
}

func formatByteCount(bytes int) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return pluralize(bytes, "byte")
	}
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	if noun == "match" {
		return fmt.Sprintf("%d matches", count)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func escapeControlChars(value string) string {
	return strings.NewReplacer(
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	).Replace(value)
}

func compactMiddle(value string, maxRunes int) string {
	if maxRunes < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	remaining := maxRunes - 1
	prefix := (remaining + 1) / 2
	suffix := remaining - prefix
	return string(runes[:prefix]) + "…" + string(runes[len(runes)-suffix:])
}

func compactDisplayValue(value string, maxRunes int) string {
	return compactMiddle(escapeControlChars(value), maxRunes)
}

func compactDisplayPath(path string) string {
	path = escapeControlChars(path)
	if utf8.RuneCountInString(path) <= maxDisplayPathRunes {
		return path
	}

	if idx := strings.LastIndexByte(path, '/'); idx > 0 && idx < len(path)-1 {
		base := path[idx+1:]
		dir := path[:idx]
		dirRunes := []rune(dir)
		baseRunes := utf8.RuneCountInString(base)
		kept := maxDisplayPathRunes - baseRunes - 4
		if kept >= 1 {
			if kept > len(dirRunes) {
				kept = len(dirRunes)
			}
			return string(dirRunes[:kept]) + "/…/" + base
		}
		if baseRunes+2 <= maxDisplayPathRunes {
			return "…/" + base
		}
		return compactMiddle(base, maxDisplayPathRunes)
	}

	return compactMiddle(path, maxDisplayPathRunes)
}

func FormatToolInput(toolName string, input map[string]any, workingDir string) string {
	return formatToolInputDetail(toolName, input, workingDir)
}

func formatMCPToolInput(input map[string]any) string {
	server, _ := input["server"].(string)
	tool, _ := input["tool"].(string)
	if server == "" || tool == "" {
		return ""
	}
	return compactDisplayValue(server+"/"+tool, maxDisplayValueRunes)
}

func formatDelegateTaskInput(input map[string]any) string {
	tasks, ok := input["tasks"].([]any)
	if !ok {
		return "0 tasks"
	}
	label := "tasks"
	if len(tasks) == 1 {
		label = "task"
	}

	agentCounts := make(map[string]int)
	for _, task := range tasks {
		task, ok := task.(map[string]any)
		if !ok {
			continue
		}
		agent, _ := task["agent"].(string)
		if agent != "" {
			agentCounts[agent]++
		}
	}
	if len(agentCounts) == 0 {
		return fmt.Sprintf("%d %s", len(tasks), label)
	}

	return fmt.Sprintf("%d %s (%s)", len(tasks), label, formatAgentCounts(agentCounts))
}

func delegateResultCounts(output any) (completed, failed int, completedByAgent, failedByAgent map[string]int, ok bool) {
	result, ok := output.(map[string]any)
	if !ok {
		return 0, 0, nil, nil, false
	}
	completed, completedOK := intValue(result["completed"])
	failed, failedOK := intValue(result["failed"])
	completedByAgent, completedAgentsOK := agentCountsValue(result["completed_by_agent"])
	failedByAgent, failedAgentsOK := agentCountsValue(result["failed_by_agent"])
	return completed, failed, completedByAgent, failedByAgent, completedOK && failedOK && completedAgentsOK && failedAgentsOK
}

func agentCountsValue(value any) (map[string]int, bool) {
	counts := make(map[string]int)
	switch value := value.(type) {
	case map[string]int:
		for agent, count := range value {
			counts[agent] = count
		}
	case map[string]any:
		for agent, value := range value {
			count, ok := intValue(value)
			if !ok {
				return nil, false
			}
			counts[agent] = count
		}
	default:
		return nil, false
	}
	return counts, true
}

func formatAgentCounts(agentCounts map[string]int) string {
	agents := make([]string, 0, len(agentCounts))
	for agent := range agentCounts {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	parts := make([]string, 0, len(agents))
	for _, agent := range agents {
		parts = append(parts, fmt.Sprintf("%s ×%d", agent, agentCounts[agent]))
	}
	return strings.Join(parts, ", ")
}

func intValue(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case float64:
		return int(value), value == float64(int(value))
	default:
		return 0, false
	}
}

func formatToolPathForUI(path, _ string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
