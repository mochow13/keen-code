package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mochow13/keen-code/internal/decision"
)

const (
	ProviderID = "typesafe"
	defaultURL = "https://api.typesafe.ai"
)

type Config struct {
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	APIKeyHelper string `json:"api_key_helper,omitempty"`
}

type Factory struct{}

func (Factory) ID() string { return ProviderID }

func (Factory) New(rawConfig json.RawMessage) (decision.Evaluator, error) {
	var cfg Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("decode TypeSafe decision config: %w", err)
		}
	}
	return New(cfg)
}

type Evaluator struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func New(cfg Config) (*Evaluator, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for %s", ProviderID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultURL
	}
	return &Evaluator{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 3 * time.Second},
	}, nil
}

func (e *Evaluator) ID() string { return ProviderID }

func (e *Evaluator) Evaluate(ctx context.Context, req decision.Request) (decision.Response, error) {
	if e == nil {
		return decision.Response{}, fmt.Errorf("TypeSafe evaluator is not initialized")
	}
	if !json.Valid(req.State) {
		return decision.Response{}, fmt.Errorf("decision state must be valid JSON")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return decision.Response{}, fmt.Errorf("decision model is required")
	}
	payload, err := marshalRequest(model, req)
	if err != nil {
		return decision.Response{}, err
	}

	response, err := e.evaluateOnce(ctx, payload)
	if err != nil {
		return decision.Response{}, err
	}
	if err := validateResponse(response, req.Questions); err != nil {
		return decision.Response{}, err
	}
	return response, nil
}

func marshalRequest(model string, req decision.Request) ([]byte, error) {
	var state any
	if err := json.Unmarshal(req.State, &state); err != nil {
		return nil, fmt.Errorf("decode decision state: %w", err)
	}
	questions := make(map[string]any, len(req.Questions))
	for id, question := range req.Questions {
		if id == "" || question.Type == "" || question.Instructions == nil {
			return nil, fmt.Errorf("invalid decision question %q", id)
		}
		body := map[string]any{"type": question.Type, "instructions": question.Instructions}
		if question.Criteria != nil {
			body["criteria"] = question.Criteria
		}
		questions[id] = body
	}
	return json.Marshal(map[string]any{"model": model, "state": state, "questions": questions})
}

func (e *Evaluator) evaluateOnce(ctx context.Context, payload []byte) (decision.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/systemone", bytes.NewReader(payload))
	if err != nil {
		return decision.Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	response, err := e.client.Do(httpReq)
	if err != nil {
		return decision.Response{}, fmt.Errorf("call TypeSafe API: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return decision.Response{}, fmt.Errorf("read TypeSafe response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decision.Response{}, fmt.Errorf("TypeSafe API returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseResponse(body)
}

type apiResponse struct {
	Model   string               `json:"model"`
	Answers map[string]apiAnswer `json:"answers"`
	Usage   apiUsage             `json:"usage"`
}

type apiUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type apiAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    *float64           `json:"confidence"`
	Noul          *float64           `json:"noul"`
	Score         *float64           `json:"score"`
	Legend        map[string]string  `json:"legend"`
}

func parseResponse(body []byte) (decision.Response, error) {
	var raw apiResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return decision.Response{}, fmt.Errorf("decode TypeSafe response: %w", err)
	}
	if raw.Model == "" || raw.Answers == nil {
		return decision.Response{}, fmt.Errorf("invalid TypeSafe response")
	}
	result := decision.Response{Model: raw.Model, Usage: decision.Usage{InputTokens: raw.Usage.InputTokens, OutputTokens: raw.Usage.OutputTokens}, Answers: make(map[string]decision.Answer, len(raw.Answers))}
	for id, answer := range raw.Answers {
		converted := decision.Answer{Type: answer.Type, Choice: answer.Choice, Probabilities: answer.Probabilities, Legend: answer.Legend}
		if answer.Confidence != nil {
			converted.Confidence = *answer.Confidence
		}
		if answer.Noul != nil {
			converted.Noul = *answer.Noul
		}
		if answer.Score != nil {
			converted.Score = *answer.Score
		}
		result.Answers[id] = converted
	}
	return result, nil
}

func validateResponse(response decision.Response, questions map[string]decision.Question) error {
	if len(response.Answers) != len(questions) {
		return fmt.Errorf("TypeSafe response answer count does not match request")
	}
	for id, question := range questions {
		answer, ok := response.Answers[id]
		if !ok || answer.Type != question.Type {
			return fmt.Errorf("invalid TypeSafe answer for question %q", id)
		}
		switch answer.Type {
		case decision.QuestionChoice:
			if answer.Choice == "" || len(answer.Probabilities) == 0 || !probability(answer.Confidence) {
				return fmt.Errorf("invalid TypeSafe choice answer for question %q", id)
			}
			for _, value := range answer.Probabilities {
				if !probability(value) {
					return fmt.Errorf("invalid TypeSafe choice probability for question %q", id)
				}
			}
		case decision.QuestionNoul:
			if !probability(answer.Noul) {
				return fmt.Errorf("invalid TypeSafe noul answer for question %q", id)
			}
		case decision.QuestionScore:
			if math.IsNaN(answer.Score) || math.IsInf(answer.Score, 0) || !probability(answer.Confidence) {
				return fmt.Errorf("invalid TypeSafe score answer for question %q", id)
			}
		default:
			return fmt.Errorf("unsupported TypeSafe answer type %q", answer.Type)
		}
	}
	return nil
}

func probability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
