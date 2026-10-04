package taskcomplexity

import (
	"context"
	"fmt"

	"github.com/mochow13/keen-code/internal/decision"
)

type Category string

const (
	CategorySimple   Category = "simple"
	CategoryStandard Category = "standard"
	CategoryComplex  Category = "complex"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Input struct {
	Messages  []Message
	Cwd       string
	GitBranch string
}

type Result struct {
	Category      Category
	Probability   float64
	Confidence    float64
	Consequential float64
	Model         string
	Usage         decision.Usage
}

type Classifier struct {
	evaluator decision.Evaluator
	model     string
}

func New(evaluator decision.Evaluator, model string) (*Classifier, error) {
	if evaluator == nil {
		return nil, fmt.Errorf("decision evaluator is required")
	}
	return &Classifier{evaluator: evaluator, model: model}, nil
}

func (c *Classifier) Classify(ctx context.Context, input Input) (Result, error) {
	request, err := buildRequest(c.model, input)
	if err != nil {
		return Result{}, err
	}
	response, err := c.evaluator.Evaluate(ctx, request)
	if err != nil {
		return Result{}, err
	}
	return parseResponse(response)
}

func (c *Classifier) EvaluatorID() string {
	if c == nil || c.evaluator == nil {
		return ""
	}
	return c.evaluator.ID()
}
