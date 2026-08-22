package llm

import (
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/firebase/genkit/go/ai"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

func TestContextFitsBudget(t *testing.T) {
	if !contextFitsBudget(13050, 700) {
		t.Fatal("expected context to fit budget")
	}
	if contextFitsBudget(13050, 9000) {
		t.Fatal("expected context to exceed budget")
	}
}

func TestReduceToolResultsForRequest_RemovesOldestUntilBudgetFits(t *testing.T) {
	var removed []int
	targets := []toolResultReductionTarget{
		{tokenCount: 50, remove: func() { removed = append(removed, 0) }},
		{tokenCount: 100, remove: func() { removed = append(removed, 1) }},
		{tokenCount: 100, remove: func() { removed = append(removed, 2) }},
	}

	reduction := reduceToolResultsForRequest(contextWindowForInputBudget(800), 850, targets)

	if !reduction.FitsBudget {
		t.Fatal("expected reduced context to fit budget")
	}
	if reduction.RemovedToolResults != 2 {
		t.Fatalf("expected 2 removed tool results, got %d", reduction.RemovedToolResults)
	}
	if reduction.OriginalTokenCount != 850 {
		t.Fatalf("expected original token count 850, got %d", reduction.OriginalTokenCount)
	}
	wantReduced := 850 - 50 - 100 + 2*estimateContextTokenCount(removedToolResultPlaceholder)
	if reduction.ReducedTokenCount != wantReduced {
		t.Fatalf("expected reduced token count %d, got %d", wantReduced, reduction.ReducedTokenCount)
	}
	if len(removed) != 2 || removed[0] != 0 || removed[1] != 1 {
		t.Fatalf("expected oldest two removals, got %v", removed)
	}
}

func TestReduceToolResultsForRequest_ReturnsNotFitAfterRemovingAllTargets(t *testing.T) {
	removed := 0
	targets := []toolResultReductionTarget{
		{tokenCount: 50, remove: func() { removed++ }},
	}

	reduction := reduceToolResultsForRequest(contextWindowForInputBudget(800), 1000, targets)

	if reduction.FitsBudget {
		t.Fatal("expected reduced context to remain over budget")
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if removed != 1 {
		t.Fatalf("expected target removal to run once, got %d", removed)
	}
}

func TestReduceToolResultsForRequest_SkipsTargetsBelowThreshold(t *testing.T) {
	var removed []int
	placeholderTokenCount := estimateContextTokenCount(removedToolResultPlaceholder)
	targets := []toolResultReductionTarget{
		{tokenCount: 5, remove: func() { removed = append(removed, 0) }},
		{tokenCount: placeholderTokenCount + 1, remove: func() { removed = append(removed, 1) }},
	}

	reduction := reduceToolResultsForRequest(contextWindowForInputBudget(800), 801, targets)

	if !reduction.FitsBudget {
		t.Fatal("expected reduced context to fit budget")
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if len(removed) != 1 || removed[0] != 1 {
		t.Fatalf("expected only large target removal, got %v", removed)
	}
}

func TestReduceOpenAIContextForRequest_ReplacesOldestToolResults(t *testing.T) {
	longResult := repeatString("old ", 200)
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("inspect files"),
		openai.ToolMessage(longResult, "call_old"),
		openai.ToolMessage("recent result", "call_recent"),
	}
	originalTokenCount := estimateOpenAIMessagesTokenCount(messages)
	budget := originalTokenCount - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceOpenAIContextForRequest(contextWindowForInputBudget(budget), messages)

	if !reduction.FitsBudget {
		t.Fatal("expected reduced context to fit budget")
	}
	if reduction.OriginalTokenCount != originalTokenCount {
		t.Fatalf("expected original token count %d, got %d", originalTokenCount, reduction.OriginalTokenCount)
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if got := openAIToolContent(reduced[1].OfTool.Content); got != removedToolResultPlaceholder {
		t.Fatalf("expected oldest tool result placeholder, got %q", got)
	}
	if got := openAIToolContent(reduced[2].OfTool.Content); got != "recent result" {
		t.Fatalf("expected recent tool result to remain, got %q", got)
	}
}

func TestReduceOpenAIContextForRequest_UsesFullMessageEstimate(t *testing.T) {
	longResult := repeatString("new ", 200)
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("inspect files"),
		openai.ToolMessage(longResult, "call_new"),
	}
	originalTokenCount := estimateOpenAIMessagesTokenCount(messages)
	budget := originalTokenCount - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceOpenAIContextForRequest(contextWindowForInputBudget(budget), messages)

	if !reduction.FitsBudget {
		t.Fatal("expected reduced context to fit budget")
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if got := openAIToolContent(reduced[1].OfTool.Content); got != removedToolResultPlaceholder {
		t.Fatalf("expected tool result placeholder, got %q", got)
	}
}

func TestReduceOpenAIContextForRequest_SkipsReductionWhenFullEstimateFits(t *testing.T) {
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("inspect files"),
		openai.ToolMessage("old result", "call_old"),
		openai.ToolMessage("new result", "call_new"),
	}
	budget := estimateOpenAIMessagesTokenCount(messages) + 10

	reduced, reduction := reduceOpenAIContextForRequest(contextWindowForInputBudget(budget), messages)

	if !reduction.FitsBudget {
		t.Fatal("expected context to fit budget")
	}
	if reduction.RemovedToolResults != 0 {
		t.Fatalf("expected no removed tool results, got %d", reduction.RemovedToolResults)
	}
	if got := openAIToolContent(reduced[1].OfTool.Content); got != "old result" {
		t.Fatalf("expected old tool result to remain, got %q", got)
	}
}

func TestReduceResponsesContextForRequest_SkipsExistingPlaceholders(t *testing.T) {
	longResult := repeatString("old ", 200)
	input := []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfFunctionCallOutput("call_removed", removedToolResultPlaceholder),
		responses.ResponseInputItemParamOfFunctionCallOutput("call_old", longResult),
		responses.ResponseInputItemParamOfFunctionCallOutput("call_recent", "recent result"),
	}
	budget := estimateResponsesInputTokenCount(input) - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceResponsesContextForRequest(contextWindowForInputBudget(budget), input)

	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	if got := reduced[0].OfFunctionCallOutput.Output.OfString.Value; got != removedToolResultPlaceholder {
		t.Fatalf("expected existing placeholder to remain, got %q", got)
	}
	if got := reduced[1].OfFunctionCallOutput.Output.OfString.Value; got != removedToolResultPlaceholder {
		t.Fatalf("expected oldest non-placeholder result to be replaced, got %q", got)
	}
	if got := reduced[2].OfFunctionCallOutput.Output.OfString.Value; got != "recent result" {
		t.Fatalf("expected recent tool result to remain, got %q", got)
	}
}

func TestReduceResponsesContextForRequest_ReplacesArrayOutput(t *testing.T) {
	longResult := repeatString("old ", 200)
	arrayOutput := responses.ResponseInputItemUnionParam{
		OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
			CallID: "call_old",
			Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
				OfResponseFunctionCallOutputItemArray: responses.ResponseFunctionCallOutputItemListParam{
					responses.ResponseFunctionCallOutputItemParamOfInputText(longResult),
				},
			},
		},
	}
	input := []responses.ResponseInputItemUnionParam{
		arrayOutput,
		responses.ResponseInputItemParamOfFunctionCallOutput("call_recent", "recent result"),
	}
	arrayContent, ok := responsesToolOutputContent(arrayOutput.OfFunctionCallOutput.Output)
	if !ok {
		t.Fatal("expected array-backed output content")
	}
	budget := estimateResponsesInputTokenCount(input) - estimateContextTokenCount(arrayContent)/2

	reduced, reduction := reduceResponsesContextForRequest(contextWindowForInputBudget(budget), input)

	if !reduction.FitsBudget {
		t.Fatal("expected reduced context to fit budget")
	}
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	output := reduced[0].OfFunctionCallOutput.Output
	if got := output.OfString.Value; got != removedToolResultPlaceholder {
		t.Fatalf("expected array output to be replaced, got %q", got)
	}
	if output.OfResponseFunctionCallOutputItemArray != nil {
		t.Fatal("expected array output arm to be cleared")
	}
	if got := reduced[1].OfFunctionCallOutput.Output.OfString.Value; got != "recent result" {
		t.Fatalf("expected recent tool result to remain, got %q", got)
	}
}

func TestReduceAnthropicContextForRequest_ReplacesToolResultContentOnly(t *testing.T) {
	longResult := repeatString("old ", 200)
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("toolu_old", longResult, false),
			anthropic.NewToolResultBlock("toolu_recent", "recent result", false),
		),
	}
	budget := estimateAnthropicMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceAnthropicContextForRequest(contextWindowForInputBudget(budget), messages)

	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	oldResult := reduced[0].Content[0].OfToolResult
	if oldResult == nil {
		t.Fatal("expected first block to remain a tool result")
	}
	if oldResult.ToolUseID != "toolu_old" {
		t.Fatalf("expected tool use id to be preserved, got %q", oldResult.ToolUseID)
	}
	if got := anthropicToolResultContent(oldResult); got != removedToolResultPlaceholder {
		t.Fatalf("expected placeholder content, got %q", got)
	}
	if got := anthropicToolResultContent(reduced[0].Content[1].OfToolResult); got != "recent result" {
		t.Fatalf("expected recent tool result to remain, got %q", got)
	}
}

func TestReduceGenkitContextForRequest_ReplacesToolResponseOutput(t *testing.T) {
	longResult := repeatString("old ", 200)
	messages := []*ai.Message{
		ai.NewMessage(ai.RoleTool, nil,
			ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   "read_file",
				Ref:    "call_old",
				Output: map[string]any{"content": longResult},
			}),
			ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   "grep",
				Ref:    "call_recent",
				Output: "recent result",
			}),
		),
	}
	budget := estimateGenkitMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceGenkitContextForRequest(contextWindowForInputBudget(budget), messages)

	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	oldResponse := reduced[0].Content[0].ToolResponse
	if oldResponse == nil {
		t.Fatal("expected first part to remain a tool response")
	}
	if oldResponse.Name != "read_file" || oldResponse.Ref != "call_old" {
		t.Fatalf("expected tool response identity to be preserved, got name=%q ref=%q", oldResponse.Name, oldResponse.Ref)
	}
	if got := oldResponse.Output; got != removedToolResultPlaceholder {
		t.Fatalf("expected placeholder output, got %#v", got)
	}
	if got := reduced[0].Content[1].ToolResponse.Output; got != "recent result" {
		t.Fatalf("expected recent tool output to remain, got %#v", got)
	}
}

func TestReduceGenkitContextForRequest_PreservesAskUserResult(t *testing.T) {
	longResult := repeatString("answer ", 200)
	messages := []*ai.Message{
		ai.NewMessage(ai.RoleModel, nil,
			ai.NewToolRequestPart(&ai.ToolRequest{Name: "ask_user", Ref: "ask"}),
			ai.NewToolRequestPart(&ai.ToolRequest{Name: "read_file", Ref: "read"}),
		),
		ai.NewMessage(ai.RoleTool, nil,
			ai.NewToolResponsePart(&ai.ToolResponse{Name: "ask_user", Ref: "ask", Output: map[string]any{"answers": []string{longResult}, "cancelled": false}}),
			ai.NewToolResponsePart(&ai.ToolResponse{Name: "read_file", Ref: "read", Output: map[string]any{"content": longResult}}),
		),
	}
	budget := estimateGenkitMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2
	reduced, reduction := reduceGenkitContextForRequest(contextWindowForInputBudget(budget), messages)
	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected one removable tool result, got %d", reduction.RemovedToolResults)
	}
	if reduced[1].Content[0].ToolResponse.Output == removedToolResultPlaceholder {
		t.Fatal("ask_user result was pruned")
	}
	if reduced[1].Content[1].ToolResponse.Output != removedToolResultPlaceholder {
		t.Fatal("expected ordinary tool result to be pruned")
	}
}

func TestReduceBedrockContextForRequest_ReplacesToolResultContentOnly(t *testing.T) {
	longResult := repeatString("old ", 200)
	messages := []brtypes.Message{
		{
			Role: brtypes.ConversationRoleUser,
			Content: []brtypes.ContentBlock{
				&brtypes.ContentBlockMemberToolResult{Value: brtypes.ToolResultBlock{
					ToolUseId: aws.String("toolu_old"),
					Content: []brtypes.ToolResultContentBlock{
						&brtypes.ToolResultContentBlockMemberText{Value: longResult},
					},
					Status: brtypes.ToolResultStatusSuccess,
				}},
				&brtypes.ContentBlockMemberToolResult{Value: brtypes.ToolResultBlock{
					ToolUseId: aws.String("toolu_recent"),
					Content: []brtypes.ToolResultContentBlock{
						&brtypes.ToolResultContentBlockMemberText{Value: "recent result"},
					},
					Status: brtypes.ToolResultStatusSuccess,
				}},
				&brtypes.ContentBlockMemberText{Value: repeatString("keep ", 120)},
			},
		},
	}
	budget := estimateBedrockMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2

	reduced, reduction := reduceBedrockContextForRequest(contextWindowForInputBudget(budget), messages)

	if reduction.RemovedToolResults != 1 {
		t.Fatalf("expected 1 removed tool result, got %d", reduction.RemovedToolResults)
	}
	oldResult := reduced[0].Content[0].(*brtypes.ContentBlockMemberToolResult)
	if aws.ToString(oldResult.Value.ToolUseId) != "toolu_old" {
		t.Fatalf("expected tool use id to be preserved, got %q", aws.ToString(oldResult.Value.ToolUseId))
	}
	if oldResult.Value.Status != brtypes.ToolResultStatusSuccess {
		t.Fatalf("expected status to be preserved, got %q", oldResult.Value.Status)
	}
	if got := bedrockToolResultContent(oldResult.Value.Content); got != removedToolResultPlaceholder {
		t.Fatalf("expected placeholder content, got %q", got)
	}
	recentResult := reduced[0].Content[1].(*brtypes.ContentBlockMemberToolResult)
	if got := bedrockToolResultContent(recentResult.Value.Content); got != "recent result" {
		t.Fatalf("expected recent tool result to remain, got %q", got)
	}
}

func TestContextReducersPreserveAskUserResultsByCallIdentity(t *testing.T) {
	longResult := repeatString("result ", 200)

	t.Run("OpenAI", func(t *testing.T) {
		assistant := openai.ChatCompletionAssistantMessageParam{ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
			{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{ID: "ask", Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: "ask_user", Arguments: `{}`}}},
			{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{ID: "read", Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: "read_file", Arguments: `{}`}}},
		}}
		messages := []openai.ChatCompletionMessageParamUnion{{OfAssistant: &assistant}, openai.ToolMessage(longResult, "ask"), openai.ToolMessage(longResult, "read")}
		budget := estimateOpenAIMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2
		reduced, reduction := reduceOpenAIContextForRequest(contextWindowForInputBudget(budget), messages)
		if reduction.RemovedToolResults != 1 || openAIToolContent(reduced[1].OfTool.Content) != longResult || openAIToolContent(reduced[2].OfTool.Content) != removedToolResultPlaceholder {
			t.Fatalf("unexpected reduction: %#v", reduction)
		}
	})

	t.Run("Responses", func(t *testing.T) {
		input := []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfFunctionCall(`{}`, "ask", "ask_user"), responses.ResponseInputItemParamOfFunctionCallOutput("ask", longResult),
			responses.ResponseInputItemParamOfFunctionCall(`{}`, "read", "read_file"), responses.ResponseInputItemParamOfFunctionCallOutput("read", longResult),
		}
		budget := estimateResponsesInputTokenCount(input) - estimateContextTokenCount(longResult)/2
		reduced, reduction := reduceResponsesContextForRequest(contextWindowForInputBudget(budget), input)
		if reduction.RemovedToolResults != 1 || reduced[1].OfFunctionCallOutput.Output.OfString.Value != longResult || reduced[3].OfFunctionCallOutput.Output.OfString.Value != removedToolResultPlaceholder {
			t.Fatalf("unexpected reduction: %#v", reduction)
		}
	})

	t.Run("Anthropic", func(t *testing.T) {
		messages := []anthropic.MessageParam{
			anthropic.NewAssistantMessage(anthropic.NewToolUseBlock("ask", map[string]any{}, "ask_user"), anthropic.NewToolUseBlock("read", map[string]any{}, "read_file")),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock("ask", longResult, false), anthropic.NewToolResultBlock("read", longResult, false)),
		}
		budget := estimateAnthropicMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2
		reduced, reduction := reduceAnthropicContextForRequest(contextWindowForInputBudget(budget), messages)
		if reduction.RemovedToolResults != 1 || anthropicToolResultContent(reduced[1].Content[0].OfToolResult) != longResult || anthropicToolResultContent(reduced[1].Content[1].OfToolResult) != removedToolResultPlaceholder {
			t.Fatalf("unexpected reduction: %#v", reduction)
		}
	})

	t.Run("Bedrock", func(t *testing.T) {
		messages := []brtypes.Message{
			{Role: brtypes.ConversationRoleAssistant, Content: []brtypes.ContentBlock{
				&brtypes.ContentBlockMemberToolUse{Value: brtypes.ToolUseBlock{ToolUseId: aws.String("ask"), Name: aws.String("ask_user")}},
				&brtypes.ContentBlockMemberToolUse{Value: brtypes.ToolUseBlock{ToolUseId: aws.String("read"), Name: aws.String("read_file")}},
			}},
			{Role: brtypes.ConversationRoleUser, Content: []brtypes.ContentBlock{
				&brtypes.ContentBlockMemberToolResult{Value: brtypes.ToolResultBlock{ToolUseId: aws.String("ask"), Content: []brtypes.ToolResultContentBlock{&brtypes.ToolResultContentBlockMemberText{Value: longResult}}}},
				&brtypes.ContentBlockMemberToolResult{Value: brtypes.ToolResultBlock{ToolUseId: aws.String("read"), Content: []brtypes.ToolResultContentBlock{&brtypes.ToolResultContentBlockMemberText{Value: longResult}}}},
			}},
		}
		budget := estimateBedrockMessagesTokenCount(messages) - estimateContextTokenCount(longResult)/2
		reduced, reduction := reduceBedrockContextForRequest(contextWindowForInputBudget(budget), messages)
		ask := reduced[1].Content[0].(*brtypes.ContentBlockMemberToolResult)
		read := reduced[1].Content[1].(*brtypes.ContentBlockMemberToolResult)
		if reduction.RemovedToolResults != 1 || bedrockToolResultContent(ask.Value.Content) != longResult || bedrockToolResultContent(read.Value.Content) != removedToolResultPlaceholder {
			t.Fatalf("unexpected reduction: %#v", reduction)
		}
	})
}

func contextWindowForInputBudget(budget int) int {
	if budget+4096 < 81920 {
		return budget + 4096
	}
	return (20*budget + 18) / 19
}

func repeatString(s string, count int) string {
	var out strings.Builder
	for range count {
		out.WriteString(s)
	}
	return out.String()
}
