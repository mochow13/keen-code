package taskcomplexity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/decision"
)

type fakeEvaluator struct {
	request  decision.Request
	response decision.Response
}

func (f *fakeEvaluator) ID() string { return "fake" }
func (f *fakeEvaluator) Evaluate(_ context.Context, request decision.Request) (decision.Response, error) {
	f.request = request
	return f.response, nil
}

func TestClassifierBuildsPayloadAndParsesResult(t *testing.T) {
	evaluator := &fakeEvaluator{response: decision.Response{
		Model: "jev-test",
		Answers: map[string]decision.Answer{
			"category":      {Type: decision.QuestionChoice, Choice: "standard", Probabilities: map[string]float64{"standard": 0.62}, Confidence: 0.4},
			"consequential": {Type: decision.QuestionNoul, Noul: 0.81},
		},
		Usage: decision.Usage{InputTokens: 12, OutputTokens: 3},
	}}
	classifier, err := New(evaluator, "model")
	if err != nil {
		t.Fatal(err)
	}
	result, err := classifier.Classify(context.Background(), Input{
		Cwd:       "/work",
		GitBranch: "main",
		Messages:  []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}, {Role: "user", Content: "implement it"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != CategoryStandard || result.Probability != 0.62 || result.Consequential != 0.81 || result.Model != "jev-test" {
		t.Fatalf("result = %#v", result)
	}
	var state struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(evaluator.request.State, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 3 || state.Messages[2].Content != "implement it" {
		t.Fatalf("state = %#v", state)
	}
}

func TestClassifierDropsOldMessagesToFitBudget(t *testing.T) {
	evaluator := &fakeEvaluator{response: decision.Response{Model: "model", Answers: map[string]decision.Answer{
		"category":      {Type: decision.QuestionChoice, Choice: "simple", Probabilities: map[string]float64{"simple": 1}, Confidence: 1},
		"consequential": {Type: decision.QuestionNoul, Noul: 0},
	}}}
	classifier, _ := New(evaluator, "")
	_, err := classifier.Classify(context.Background(), Input{Messages: []Message{
		{Role: "user", Content: strings.Repeat("old", 30000)},
		{Role: "assistant", Content: strings.Repeat("old", 30000)},
		{Role: "user", Content: "latest"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(evaluator.request.State, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 || state.Messages[0].Content != "latest" {
		t.Fatalf("state = %#v", state)
	}
}
