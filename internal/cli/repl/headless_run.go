package repl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/session"
)

const (
	HeadlessFormatText = "text"
	HeadlessFormatJSON = "json"
)

// ErrCompletionSignalMissing is returned when a --completion-signal was
// configured but the final response did not contain it. The caller should
// continue the loop with another iteration.
var ErrCompletionSignalMissing = errors.New("completion signal not found in response")

type HeadlessRunOptions struct {
	WorkingDir       string
	Config           *config.ResolvedConfig
	GlobalConfig     *config.GlobalConfig
	MCPRuntime       keenmcp.Runtime
	Client           llm.LLMClient
	SessionID        string
	Prompt           string
	Format           string
	CompletionSignal string
	Out              io.Writer
	// Progress, when set and Format is text, receives live text chunks and
	// tool end lines as the run happens.
	Progress io.Writer
}

type HeadlessRunResult struct {
	SessionID         string         `json:"session_id"`
	OpenCodeSessionID string         `json:"opencode_session_id"`
	Text              string         `json:"text"`
	Usage             *headlessUsage `json:"usage,omitempty"`
}

type headlessUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
	TotalTokens     int `json:"total_tokens"`
	CachedTokens    int `json:"cached_tokens"`
}

func RunHeadless(ctx context.Context, opts HeadlessRunOptions) (*HeadlessRunResult, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if opts.Client == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}
	format := opts.Format
	if format == "" {
		format = HeadlessFormatText
	}
	if format != HeadlessFormatText && format != HeadlessFormatJSON {
		return nil, fmt.Errorf("unsupported format %q", format)
	}

	progress := newHeadlessProgress(nil, "")
	if format == HeadlessFormatText && opts.Progress != nil {
		progress = newHeadlessProgress(opts.Progress, opts.WorkingDir)
	}
	defer progress.newLine()

	appState := replappstate.New(opts.Client, opts.WorkingDir)
	permissionRequester := replpermissions.NewAutoApproveRequester()
	diffEmitter := repltooling.NewDiffEmitter()
	repltooling.SetupToolRegistry(opts.WorkingDir, appState, permissionRequester, diffEmitter, opts.MCPRuntime, opts.Config, opts.GlobalConfig, nil)

	sessions := newReplSessionState(opts.WorkingDir)
	if sessions == nil {
		return nil, fmt.Errorf("session store unavailable")
	}
	if opts.SessionID != "" {
		loaded, err := loadHeadlessSession(sessions, opts.SessionID)
		if err != nil {
			return nil, err
		}
		appState.ReplaceMessages(session.BuildConversation(loaded.Events))
	}

	if err := sessions.appendUserMessage(prompt); err != nil {
		return nil, err
	}
	appState.AddMessage(llm.RoleUser, prompt)

	eventCh, err := appState.StreamChat(ctx, opts.Config, llm.StreamOptions{SessionID: sessions.currentID()})
	if err != nil {
		return nil, err
	}
	if eventCh == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	handler := NewStreamHandler(nil)
	handler.workingDir = opts.WorkingDir
	handler.showThinking = false
	handler.Start(eventCh, "")
	turnMemory := newTurnMemoryAccumulator(false)
	var completedText strings.Builder

	var lastUsage *llm.TokenUsage
	for {
		select {
		case diffReq := <-diffEmitter.GetDiffChan():
			handler.HandleDiff(diffReq.Lines)
			close(diffReq.Done)
		case event, ok := <-eventCh:
			if !ok {
				return finishHeadlessRun(opts.Out, format, opts.CompletionSignal, sessions, handler, turnMemory, completedText.String(), lastUsage)
			}
			switch event.Type {
			case llm.StreamEventTypeChunk:
				handler.HandleChunk(event.Content)
				progress.writeText(event.Content)
			case llm.StreamEventTypeReasoningChunk:
				handler.HandleReasoningChunk(event.Content)
			case llm.StreamEventTypeToolStart:
				handleHeadlessToolStart(handler, event.ToolCall)
				progress.newLine()
			case llm.StreamEventTypeToolEnd:
				handleHeadlessToolEnd(handler, event.ToolCall)
				progress.writeToolEnd(event.ToolCall)
			case llm.StreamEventTypeUsage:
				lastUsage = event.Usage
			case llm.StreamEventTypeRetry:
				handler.RewindForRetry()
				progress.newLine()
			case llm.StreamEventTypeAutoCompactionApplied:
				progress.newLine()
				if err := checkpointHeadlessAutoCompaction(
					sessions,
					appState,
					handler,
					turnMemory,
					&completedText,
					event.AutoCompaction,
				); err != nil {
					return nil, err
				}
				lastUsage = nil
			case llm.StreamEventTypeDone:
				return finishHeadlessRun(opts.Out, format, opts.CompletionSignal, sessions, handler, turnMemory, completedText.String(), lastUsage)
			case llm.StreamEventTypeIncomplete:
				return failHeadlessRun(opts.Out, format, sessions, handler, turnMemory, completedText.String(), lastUsage, event.Error)
			case llm.StreamEventTypeError:
				return failHeadlessRun(opts.Out, format, sessions, handler, turnMemory, completedText.String(), lastUsage, event.Error)
			}
		case <-ctx.Done():
			return failHeadlessRun(opts.Out, format, sessions, handler, turnMemory, completedText.String(), lastUsage, ctx.Err())
		}
	}
}

func loadHeadlessSession(sessions *replSessionState, sessionID string) (*session.LoadedSession, error) {
	summaries, err := sessions.listSessions()
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if summary.ID == sessionID {
			return sessions.load(summary)
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}

func handleHeadlessToolStart(handler *StreamHandler, toolCall *llm.ToolCall) {
	if toolCall == nil {
		return
	}
	if toolCall.Name == "bash" {
		command, _ := toolCall.Input["command"].(string)
		summary, _ := toolCall.Input["summary"].(string)
		handler.HandleBashStart(command, summary)
		return
	}
	handler.HandleToolStart(toolCall)
}

func handleHeadlessToolEnd(handler *StreamHandler, toolCall *llm.ToolCall) {
	if toolCall == nil {
		return
	}
	toolCall = sanitizeDelegateToolCall(toolCall)
	if toolCall.Name == "bash" {
		handler.HandleBashEnd(toolCall)
		return
	}
	handler.HandleToolEnd(toolCall)
}

func checkpointHeadlessAutoCompaction(
	sessions *replSessionState,
	appState *replappstate.AppState,
	handler *StreamHandler,
	turnMemory *turnMemoryAccumulator,
	completedText *strings.Builder,
	compaction *llm.AutoCompactionEvent,
) error {
	if compaction == nil || len(compaction.Replacement) == 0 {
		return fmt.Errorf("automatic compaction applied without replacement history")
	}

	segments := cloneStreamSegments(handler.segments)
	turnMemory.RecordToolActivity(segments, handler.workingDir)
	response := handler.GetResponse()
	persistedReplacement := appstate.WithoutSystemMessages(compaction.Replacement)
	if err := sessions.appendAutoCompaction(segments, llm.Message{
		Role:       llm.RoleAssistant,
		Content:    response,
		TurnMemory: turnMemory.Build(),
	}, persistedReplacement); err != nil {
		return err
	}

	completedText.WriteString(response)
	appState.ReplaceMessages(persistedReplacement)
	handler.ResetContent()
	*turnMemory = *newTurnMemoryAccumulator(false)
	return nil
}

func finishHeadlessRun(
	out io.Writer,
	format string,
	completionSignal string,
	sessions *replSessionState,
	handler *StreamHandler,
	turnMemory *turnMemoryAccumulator,
	completedText string,
	usage *llm.TokenUsage,
) (*HeadlessRunResult, error) {
	segments := cloneStreamSegments(handler.segments)
	turnMemory.RecordToolActivity(segments, handler.workingDir)
	_, currentResponse := handler.HandleDone()
	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    currentResponse,
		TurnMemory: turnMemory.Build(),
	}
	if err := sessions.appendAssistantTurn(segments, assistantMessage, false, ""); err != nil {
		return nil, err
	}

	result := &HeadlessRunResult{
		SessionID:         sessions.currentID(),
		OpenCodeSessionID: strings.ReplaceAll(sessions.currentID(), "-", ""),
		Text:              completedText + currentResponse,
		Usage:             cloneHeadlessUsage(usage),
	}
	if err := writeHeadlessResult(out, format, result); err != nil {
		return result, err
	}
	if completionSignal != "" && !strings.Contains(result.Text, completionSignal) {
		return result, ErrCompletionSignalMissing
	}
	return result, nil
}

func failHeadlessRun(
	out io.Writer,
	format string,
	sessions *replSessionState,
	handler *StreamHandler,
	turnMemory *turnMemoryAccumulator,
	completedText string,
	usage *llm.TokenUsage,
	err error,
) (*HeadlessRunResult, error) {
	if err == nil {
		err = fmt.Errorf("LLM stream incomplete")
	}
	segments := cloneStreamSegments(handler.segments)
	turnMemory.RecordToolActivity(segments, handler.workingDir)
	partialResponse := handler.GetResponse()
	_, errMsg := handler.HandleError(err)
	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    partialResponse,
		TurnMemory: turnMemory.Build(),
	}
	_ = sessions.appendAssistantTurn(segments, assistantMessage, false, errMsg)
	result := &HeadlessRunResult{
		SessionID:         sessions.currentID(),
		OpenCodeSessionID: strings.ReplaceAll(sessions.currentID(), "-", ""),
		Text:              completedText + partialResponse,
		Usage:             cloneHeadlessUsage(usage),
	}
	if writeErr := writeHeadlessResult(out, format, result); writeErr != nil {
		return result, fmt.Errorf("%w; failed to write partial result: %v", err, writeErr)
	}
	return result, err
}

func cloneHeadlessUsage(usage *llm.TokenUsage) *headlessUsage {
	if usage == nil {
		return nil
	}
	return &headlessUsage{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		ReasoningTokens: usage.ReasoningTokens,
		TotalTokens:     usage.TotalTokens,
		CachedTokens:    usage.CachedTokens,
	}
}

func writeHeadlessResult(out io.Writer, format string, result *HeadlessRunResult) error {
	if out == nil {
		return nil
	}
	switch format {
	case HeadlessFormatJSON:
		encoder := json.NewEncoder(out)
		return encoder.Encode(result)
	default:
		if result.Text == "" {
			_, err := fmt.Fprintln(out)
			return err
		}
		_, err := fmt.Fprintln(out, result.Text)
		return err
	}
}
