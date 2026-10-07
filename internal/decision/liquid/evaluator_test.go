package liquid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/decision"
)

func TestFactoryID(t *testing.T) {
	if (Factory{}).ID() != "liquid" {
		t.Fatalf("factory ID = %q", (Factory{}).ID())
	}
}

func TestFactoryRequiresAPIKey(t *testing.T) {
	if _, err := (Factory{}).New(json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "no API key configured for liquid") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestFactoryEvaluate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer liquid_test-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "d1",
			"answers": map[string]any{"is_complaint": map[string]any{"type": "noul", "noul": 0.99}},
			"usage":   map[string]int{"input_tokens": 84, "output_tokens": 0},
		})
	}))
	defer server.Close()

	evaluator, err := (Factory{}).New(json.RawMessage(`{"api_key":"liquid_test-key","base_url":"` + server.URL + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	if evaluator.ID() != "liquid" {
		t.Fatalf("evaluator ID = %q", evaluator.ID())
	}

	response, err := evaluator.Evaluate(context.Background(), decision.Request{
		Model: "d1",
		State: json.RawMessage(`"I have been waiting three weeks for my order."`),
		Questions: map[string]decision.Question{
			"is_complaint": {Type: decision.QuestionNoul, Instructions: "Is this message a complaint from the customer?"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Answers["is_complaint"].Noul != 0.99 || response.Usage.InputTokens != 84 {
		t.Fatalf("response = %#v", response)
	}
}

func TestFactoryRejectsInvalidConfig(t *testing.T) {
	if _, err := (Factory{}).New(json.RawMessage(`{invalid`)); err == nil {
		t.Fatal("expected decode error")
	}
}
