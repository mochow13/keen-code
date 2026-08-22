package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type askUserRequesterFunc func(context.Context, AskUserRequest) (AskUserResult, error)

func (f askUserRequesterFunc) RequestUser(ctx context.Context, request AskUserRequest) (AskUserResult, error) {
	return f(ctx, request)
}

func TestAskUserToolValidationAndResult(t *testing.T) {
	input := map[string]any{"questions": []any{map[string]any{"question": "Pick", "options": []any{"one", "two"}}}}
	tool := NewAskUserTool(askUserRequesterFunc(func(_ context.Context, request AskUserRequest) (AskUserResult, error) {
		if request.Questions[0].Options[0] != "one" {
			t.Fatal("options not preserved")
		}
		return AskUserResult{Answers: []string{"two"}}, nil
	}))
	if err := tool.ValidateInput(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.(AskUserResult).Answers[0]; got != "two" {
		t.Fatalf("answer = %q", got)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded); got != `{"answers":["two"],"cancelled":false}` {
		t.Fatalf("unexpected result contract %s", got)
	}
}

func TestAskUserToolRejectsInvalidQuestions(t *testing.T) {
	tool := NewAskUserTool(nil)
	for _, input := range []any{
		map[string]any{"questions": []any{}},
		map[string]any{"questions": []any{map[string]any{"question": "", "options": []any{"a", "b"}}}},
		map[string]any{"questions": []any{map[string]any{"question": "q", "options": []any{"a", "a"}}}},
	} {
		if err := tool.ValidateInput(context.Background(), input); err == nil {
			t.Fatalf("expected validation error for %#v", input)
		}
	}
}
