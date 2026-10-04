package decision

import (
	"context"
	"encoding/json"
)

const (
	QuestionChoice = "choice"
	QuestionNoul   = "noul"
	QuestionScore  = "score"
)

type Evaluator interface {
	ID() string
	Evaluate(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Model     string
	State     json.RawMessage
	Questions map[string]Question
}

type Question struct {
	Type         string
	Instructions any
	Criteria     any
}

type Response struct {
	Model   string
	Answers map[string]Answer
	Usage   Usage
}

type Answer struct {
	Type          string
	Choice        string
	Probabilities map[string]float64
	Confidence    float64
	Noul          float64
	Score         float64
	Legend        map[string]string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}
