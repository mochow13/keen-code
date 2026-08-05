package llm

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/firebase/genkit/go/ai"
	openai "github.com/openai/openai-go/v3"
	openaiParam "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const (
	removedToolResultPlaceholder   = "Tool result removed to fit context."
	contextWindowExceededError     = "context exceeds model window after removing tool results"
	defaultContextWindowTokenCount = 200000
)

var ErrContextWindowExceeded = errors.New("context window exceeded")

type contextReduction struct {
	OriginalTokenCount int
	ReducedTokenCount  int
	RemovedToolResults int
	FitsBudget         bool
}

type toolResultReductionTarget struct {
	tokenCount int
	remove     func()
}

func estimateContextTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return max(1, (len(text)+2)/3)
}

func contextInputBudget(contextWindowTokenCount int) int {
	window := contextWindowTokenCount
	if window <= 0 {
		window = defaultContextWindowTokenCount
	}
	return max(1, window-max(4096, window/20)) // 5% safety margin
}

func shouldAutoCompact(estimatedInputTokenCount, effectiveBudget int) bool {
	if effectiveBudget <= 0 {
		return estimatedInputTokenCount > 0
	}
	return estimatedInputTokenCount >= effectiveBudget-(effectiveBudget/10) // 90% boundary
}

func contextFitsBudget(contextWindowTokenCount int, currentInputTokenCount int) bool {
	return currentInputTokenCount <= contextInputBudget(contextWindowTokenCount)
}

func reduceToolResultsForRequest(contextWindowTokenCount int, currentInputTokenCount int, targets []toolResultReductionTarget) contextReduction {
	budget := contextInputBudget(contextWindowTokenCount)
	reduction := newContextReduction(currentInputTokenCount, budget)

	placeholderTokenCount := estimateContextTokenCount(removedToolResultPlaceholder)
	for _, target := range targets {
		if reduction.ReducedTokenCount <= budget {
			break
		}
		if target.tokenCount <= placeholderTokenCount {
			continue
		}
		target.remove()
		reduction.ReducedTokenCount -= target.tokenCount
		reduction.ReducedTokenCount += placeholderTokenCount
		reduction.RemovedToolResults++
	}

	reduction.FitsBudget = reduction.ReducedTokenCount <= budget
	slog.Debug(
		"Reduced context tool results",
		"inputTokenCount", reduction.OriginalTokenCount,
		"reducedTokenCount", reduction.ReducedTokenCount,
		"budgetTokenCount", budget,
		"removedToolResultCount", reduction.RemovedToolResults,
		"fitsBudget", reduction.FitsBudget,
	)
	return reduction
}

func newContextReduction(currentInputTokenCount int, budget int) contextReduction {
	return contextReduction{
		OriginalTokenCount: currentInputTokenCount,
		ReducedTokenCount:  currentInputTokenCount,
		FitsBudget:         currentInputTokenCount <= budget,
	}
}

func estimateJSONTokenCount(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return estimateContextTokenCount(string(b))
}

func reduceOpenAIContextForRequest(
	contextWindowTokenCount int,
	next []openai.ChatCompletionMessageParamUnion,
) ([]openai.ChatCompletionMessageParamUnion, contextReduction) {
	estimatedInputTokenCount := estimateOpenAIMessagesTokenCount(next)
	budget := contextInputBudget(contextWindowTokenCount)
	if estimatedInputTokenCount <= budget {
		return next, newContextReduction(estimatedInputTokenCount, budget)
	}

	targets := make([]toolResultReductionTarget, 0)
	for i, msg := range next {
		if msg.OfTool == nil {
			continue
		}
		content := openAIToolContent(msg.OfTool.Content)
		if content == removedToolResultPlaceholder {
			continue
		}
		idx := i
		targets = append(targets, toolResultReductionTarget{
			tokenCount: estimateContextTokenCount(content),
			remove: func() {
				next[idx].OfTool.Content.OfString = openaiParam.NewOpt(removedToolResultPlaceholder)
				next[idx].OfTool.Content.OfArrayOfContentParts = nil
			},
		})
	}

	return next, reduceToolResultsForRequest(contextWindowTokenCount, estimatedInputTokenCount, targets)
}

func estimateOpenAIMessagesTokenCount(messages []openai.ChatCompletionMessageParamUnion) int {
	tokenCount := 0
	for _, msg := range messages {
		tokenCount += estimateContextTokenCount(string(marshalContextOrEmpty(msg)))
	}
	return tokenCount
}

func openAIToolContent(content openai.ChatCompletionToolMessageParamContentUnion) string {
	if content.OfString.Valid() {
		return content.OfString.Value
	}
	return string(marshalContextOrEmpty(content))
}

func reduceResponsesContextForRequest(
	contextWindowTokenCount int,
	next []responses.ResponseInputItemUnionParam,
) ([]responses.ResponseInputItemUnionParam, contextReduction) {
	estimatedInputTokenCount := estimateResponsesInputTokenCount(next)
	budget := contextInputBudget(contextWindowTokenCount)
	if estimatedInputTokenCount <= budget {
		return next, newContextReduction(estimatedInputTokenCount, budget)
	}

	targets := make([]toolResultReductionTarget, 0)
	for i, item := range next {
		if item.OfFunctionCallOutput == nil {
			continue
		}
		content, ok := responsesToolOutputContent(item.OfFunctionCallOutput.Output)
		if !ok || content == removedToolResultPlaceholder {
			continue
		}
		idx := i
		targets = append(targets, toolResultReductionTarget{
			tokenCount: estimateContextTokenCount(content),
			remove: func() {
				next[idx].OfFunctionCallOutput.Output.OfString = openaiParam.NewOpt(removedToolResultPlaceholder)
				next[idx].OfFunctionCallOutput.Output.OfResponseFunctionCallOutputItemArray = nil
			},
		})
	}

	return next, reduceToolResultsForRequest(contextWindowTokenCount, estimatedInputTokenCount, targets)
}

func responsesToolOutputContent(output responses.ResponseInputItemFunctionCallOutputOutputUnionParam) (string, bool) {
	if output.OfString.Valid() {
		return output.OfString.Value, true
	}
	if output.OfResponseFunctionCallOutputItemArray != nil {
		return string(marshalContextOrEmpty(output.OfResponseFunctionCallOutputItemArray)), true
	}
	return "", false
}

func estimateResponsesInputTokenCount(input []responses.ResponseInputItemUnionParam) int {
	tokenCount := 0
	for _, item := range input {
		tokenCount += estimateContextTokenCount(string(marshalContextOrEmpty(item)))
	}
	return tokenCount
}

func reduceAnthropicContextForRequest(
	contextWindowTokenCount int,
	next []anthropic.MessageParam,
) ([]anthropic.MessageParam, contextReduction) {
	estimatedInputTokenCount := estimateAnthropicMessagesTokenCount(next)
	budget := contextInputBudget(contextWindowTokenCount)
	if estimatedInputTokenCount <= budget {
		return next, newContextReduction(estimatedInputTokenCount, budget)
	}

	targets := make([]toolResultReductionTarget, 0)
	for mi := range next {
		for bi, block := range next[mi].Content {
			if block.OfToolResult == nil {
				continue
			}
			content := anthropicToolResultContent(block.OfToolResult)
			if content == removedToolResultPlaceholder {
				continue
			}
			messageIdx := mi
			blockIdx := bi
			targets = append(targets, toolResultReductionTarget{
				tokenCount: estimateContextTokenCount(content),
				remove: func() {
					next[messageIdx].Content[blockIdx].OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: removedToolResultPlaceholder,
							},
						},
					}
				},
			})
		}
	}

	return next, reduceToolResultsForRequest(contextWindowTokenCount, estimatedInputTokenCount, targets)
}

func estimateAnthropicMessagesTokenCount(messages []anthropic.MessageParam) int {
	tokenCount := 0
	for _, msg := range messages {
		tokenCount += estimateContextTokenCount(string(marshalContextOrEmpty(msg)))
	}
	return tokenCount
}

func anthropicToolResultContent(result *anthropic.ToolResultBlockParam) string {
	var b strings.Builder
	for _, content := range result.Content {
		if content.OfText != nil {
			b.WriteString(content.OfText.Text)
			continue
		}
		b.Write(marshalContextOrEmpty(content))
	}
	return b.String()
}

func reduceGenkitContextForRequest(
	contextWindowTokenCount int,
	next []*ai.Message,
) ([]*ai.Message, contextReduction) {
	estimatedInputTokenCount := estimateGenkitMessagesTokenCount(next)
	budget := contextInputBudget(contextWindowTokenCount)
	if estimatedInputTokenCount <= budget {
		return next, newContextReduction(estimatedInputTokenCount, budget)
	}

	targets := make([]toolResultReductionTarget, 0)
	for mi, msg := range next {
		if msg == nil || msg.Role != ai.RoleTool {
			continue
		}
		for pi, part := range msg.Content {
			if part == nil || part.ToolResponse == nil || part.ToolResponse.Output == removedToolResultPlaceholder {
				continue
			}
			messageIdx := mi
			partIdx := pi
			targets = append(targets, toolResultReductionTarget{
				tokenCount: estimateJSONTokenCount(part.ToolResponse.Output),
				remove: func() {
					next[messageIdx].Content[partIdx].ToolResponse.Output = removedToolResultPlaceholder
				},
			})
		}
	}

	return next, reduceToolResultsForRequest(contextWindowTokenCount, estimatedInputTokenCount, targets)
}

func estimateGenkitMessagesTokenCount(messages []*ai.Message) int {
	tokenCount := 0
	for _, msg := range messages {
		tokenCount += estimateContextTokenCount(string(marshalContextOrEmpty(msg)))
	}
	return tokenCount
}

func reduceBedrockContextForRequest(
	contextWindowTokenCount int,
	next []brtypes.Message,
) ([]brtypes.Message, contextReduction) {
	estimatedInputTokenCount := estimateBedrockMessagesTokenCount(next)
	budget := contextInputBudget(contextWindowTokenCount)
	if estimatedInputTokenCount <= budget {
		return next, newContextReduction(estimatedInputTokenCount, budget)
	}

	targets := make([]toolResultReductionTarget, 0)
	for mi := range next {
		for bi, block := range next[mi].Content {
			toolResult, ok := block.(*brtypes.ContentBlockMemberToolResult)
			if !ok {
				continue
			}
			content := bedrockToolResultContent(toolResult.Value.Content)
			if content == removedToolResultPlaceholder {
				continue
			}
			messageIdx := mi
			blockIdx := bi
			targets = append(targets, toolResultReductionTarget{
				tokenCount: estimateContextTokenCount(content),
				remove: func() {
					next[messageIdx].Content[blockIdx].(*brtypes.ContentBlockMemberToolResult).Value.Content = []brtypes.ToolResultContentBlock{
						&brtypes.ToolResultContentBlockMemberText{Value: removedToolResultPlaceholder},
					}
				},
			})
		}
	}

	return next, reduceToolResultsForRequest(contextWindowTokenCount, estimatedInputTokenCount, targets)
}

func estimateBedrockMessagesTokenCount(messages []brtypes.Message) int {
	tokenCount := 0
	for _, msg := range messages {
		tokenCount += estimateContextTokenCount(string(marshalContextOrEmpty(msg)))
	}
	return tokenCount
}

func bedrockToolResultContent(content []brtypes.ToolResultContentBlock) string {
	var b strings.Builder
	for _, block := range content {
		switch v := block.(type) {
		case *brtypes.ToolResultContentBlockMemberText:
			b.WriteString(v.Value)
		default:
			b.Write(marshalContextOrEmpty(v))
		}
	}
	return b.String()
}

func marshalContextOrEmpty(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
