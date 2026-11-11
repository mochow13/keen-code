package repl

import (
	"time"

	"github.com/mochow13/keen-code/internal/agentcore"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
	"github.com/mochow13/keen-code/internal/session"
)

const interruptedPromptText = "Interrupted...what should the agent do instead?"

func buildAssistantTurnTranscript(segments []replstream.Segment) []session.TranscriptItem {
	items := make([]session.TranscriptItem, 0, len(segments))

	for _, seg := range segments {
		switch seg.Kind {
		case replstream.SegmentAssistant:
			if seg.Content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemText,
					Content: seg.Content,
				})
			}
		case replstream.SegmentReasoning:
			if seg.Content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemReasoning,
					Content: seg.Content,
				})
			}
		case replstream.SegmentToolStart:
			if seg.ToolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolStart,
					ToolStart: &session.ToolStartPayload{
						Name:  seg.ToolCall.Name,
						Input: cloneInput(seg.ToolCall.Input),
					},
				})
			}
		case replstream.SegmentToolEnd:
			if seg.ToolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolEnd,
					ToolEnd: &session.ToolEndPayload{
						Name:       seg.ToolCall.Name,
						Input:      cloneInput(seg.ToolCall.Input),
						Output:     seg.ToolCall.Output,
						Error:      seg.ToolCall.Error,
						DurationNS: seg.ToolCall.Duration.Nanoseconds(),
					},
				})
			}
		case replstream.SegmentBash:
			duration := int64(0)
			errText := ""
			if seg.ToolCall != nil {
				duration = seg.ToolCall.Duration.Nanoseconds()
				errText = seg.ToolCall.Error
			}
			items = append(items, session.TranscriptItem{
				Kind: session.TranscriptItemBash,
				Bash: &session.BashPayload{
					Command:    seg.Command,
					Summary:    seg.Summary,
					Output:     seg.Output,
					Stderr:     seg.Stderr,
					StdoutFile: seg.StdoutFile,
					StderrFile: seg.StderrFile,
					Error:      errText,
					DurationNS: duration,
					ExitCode:   reploutput.BashExitCode(seg.ToolCall),
				},
			})
		case replstream.SegmentDiff:
			if len(seg.DiffLines) > 0 {
				lines := make([]agentcore.EditDiffLine, len(seg.DiffLines))
				copy(lines, seg.DiffLines)
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemDiff,
					Diff: &session.DiffPayload{Lines: agentcore.ToToolDiffLines(lines)},
				})
			}
		}
	}

	return items
}

func cloneInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneStreamSegments(segments []replstream.Segment) []replstream.Segment {
	return replstream.CloneSegments(segments)
}

func toolCallFromPayload(payload *session.ToolStartPayload) *agentcore.ToolCall {
	if payload == nil {
		return nil
	}
	return &agentcore.ToolCall{
		Name:  payload.Name,
		Input: cloneInput(payload.Input),
	}
}

func toolCallResultFromPayload(payload *session.ToolEndPayload) *agentcore.ToolCall {
	if payload == nil {
		return nil
	}
	return &agentcore.ToolCall{
		Name:     payload.Name,
		Input:    cloneInput(payload.Input),
		Output:   payload.Output,
		Error:    payload.Error,
		Duration: time.Duration(payload.DurationNS),
	}
}
