package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mochow13/keen-code/internal/decision"
)

func TestEvaluatorEvaluate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["model"] != "test-model" {
			t.Fatalf("model = %#v", request["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"answers": map[string]any{
				"category": map[string]any{"type": "choice", "choice": "simple", "probabilities": map[string]float64{"simple": 0.8, "complex": 0.2}, "confidence": 0.6},
				"safe":     map[string]any{"type": "noul", "noul": 0.9},
			},
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	defer server.Close()

	evaluator, err := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	response, err := evaluator.Evaluate(context.Background(), decision.Request{
		Model: "test-model",
		State: json.RawMessage(`{"message":"hello"}`),
		Questions: map[string]decision.Question{
			"category": {Type: decision.QuestionChoice, Instructions: "category", Criteria: map[string]string{"simple": "small", "complex": "large"}},
			"safe":     {Type: decision.QuestionNoul, Instructions: "safe"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "test-model" || response.Answers["category"].Choice != "simple" || response.Usage.InputTokens != 10 {
		t.Fatalf("response = %#v", response)
	}
}

func TestEvaluatorRejectsMismatchedAnswerType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "test-model",
			"answers": map[string]any{"category": map[string]any{"type": "noul", "noul": 0.9}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	evaluator, err := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = evaluator.Evaluate(context.Background(), decision.Request{
		Model: "test-model",
		State: json.RawMessage(`"hello"`),
		Questions: map[string]decision.Question{
			"category": {Type: decision.QuestionChoice, Instructions: "category", Criteria: map[string]string{"simple": "small"}},
		},
	})
	if err == nil {
		t.Fatal("expected protocol validation error")
	}
}
