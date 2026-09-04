package tools

import (
	"context"
	"fmt"
	"strings"
)

type AskUserQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type AskUserRequest struct {
	Questions []AskUserQuestion `json:"questions"`
}

type AskUserResult struct {
	Answers   []string `json:"answers"`
	Cancelled bool     `json:"cancelled"`
}

type AskUserRequester interface {
	RequestUser(ctx context.Context, request AskUserRequest) (AskUserResult, error)
}

type AskUserTool struct {
	requester AskUserRequester
}

func NewAskUserTool(requester AskUserRequester) *AskUserTool {
	return &AskUserTool{requester: requester}
}

func (t *AskUserTool) Name() string { return AskUserToolName }

func (t *AskUserTool) Description() string {
	return "Ask the interactive user one or more clarification questions. The first option for each question is your recommendation and is preselected. Use only when user interaction is available."
}

func (t *AskUserTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 10,
				"items": map[string]any{"type": "object", "properties": map[string]any{
					"question": map[string]any{"type": "string", "description": "Question shown to the user"},
					"options":  map[string]any{"type": "array", "minItems": 2, "maxItems": 6, "items": map[string]any{"type": "string"}, "description": "Ordered options; the first is your recommendation."},
				}, "required": []string{"question", "options"}, "additionalProperties": false},
			},
		},
		"required": []string{"questions"}, "additionalProperties": false,
	}
}

func (t *AskUserTool) ValidateInput(_ context.Context, input any) error {
	_, err := parseAskUserRequest(input)
	return err
}

func (t *AskUserTool) Execute(ctx context.Context, input any) (any, error) {
	request, err := parseAskUserRequest(input)
	if err != nil {
		return nil, err
	}
	if t.requester == nil {
		return nil, fmt.Errorf("ask_user is unavailable without an interactive user")
	}
	return t.requester.RequestUser(ctx, request)
}

func parseAskUserRequest(input any) (AskUserRequest, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return AskUserRequest{}, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	rawQuestions, ok := params["questions"].([]any)
	if !ok {
		if _, exists := params["questions"]; !exists {
			return AskUserRequest{}, missingRequiredParameter(AskUserToolName, "questions", `{"questions":[{"question":"...","options":["recommended","alternative"]}]}`, "Provide one to ten questions")
		}
		return AskUserRequest{}, fmt.Errorf("invalid input: questions must be an array")
	}
	if len(rawQuestions) < 1 || len(rawQuestions) > 10 {
		return AskUserRequest{}, fmt.Errorf("invalid input: questions must contain 1 to 10 entries")
	}
	request := AskUserRequest{Questions: make([]AskUserQuestion, len(rawQuestions))}
	for i, raw := range rawQuestions {
		question, ok := raw.(map[string]any)
		if !ok {
			return AskUserRequest{}, fmt.Errorf("invalid input: questions[%d] must be an object", i)
		}
		text, ok := question["question"].(string)
		if !ok || strings.TrimSpace(text) == "" {
			return AskUserRequest{}, fmt.Errorf("invalid input: questions[%d].question must be a non-empty string", i)
		}
		rawOptions, ok := question["options"].([]any)
		if !ok || len(rawOptions) < 2 || len(rawOptions) > 6 {
			return AskUserRequest{}, fmt.Errorf("invalid input: questions[%d].options must contain 2 to 6 entries", i)
		}
		options := make([]string, len(rawOptions))
		seen := make(map[string]struct{}, len(rawOptions))
		for j, rawOption := range rawOptions {
			option, ok := rawOption.(string)
			if !ok || strings.TrimSpace(option) == "" {
				return AskUserRequest{}, fmt.Errorf("invalid input: questions[%d].options[%d] must be a non-empty string", i, j)
			}
			if _, duplicate := seen[option]; duplicate {
				return AskUserRequest{}, fmt.Errorf("invalid input: questions[%d].options must be unique", i)
			}
			seen[option] = struct{}{}
			options[j] = option
		}
		request.Questions[i] = AskUserQuestion{Question: text, Options: options}
	}
	return request, nil
}
