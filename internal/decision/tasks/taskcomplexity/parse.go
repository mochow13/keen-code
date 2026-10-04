package taskcomplexity

import (
	"fmt"
	"math"

	"github.com/mochow13/keen-code/internal/decision"
)

func parseResponse(response decision.Response) (Result, error) {
	categoryAnswer, ok := response.Answers["category"]
	if !ok || categoryAnswer.Type != decision.QuestionChoice {
		return Result{}, fmt.Errorf("missing category answer")
	}
	consequentialAnswer, ok := response.Answers["consequential"]
	if !ok || consequentialAnswer.Type != decision.QuestionNoul {
		return Result{}, fmt.Errorf("missing consequential answer")
	}

	category := Category(categoryAnswer.Choice)
	switch category {
	case CategorySimple, CategoryStandard, CategoryComplex:
	default:
		return Result{}, fmt.Errorf("invalid task complexity category %q", category)
	}
	probability, ok := categoryAnswer.Probabilities[categoryAnswer.Choice]
	if !ok || !validProbability(probability) || !validProbability(categoryAnswer.Confidence) || !validProbability(consequentialAnswer.Noul) {
		return Result{}, fmt.Errorf("invalid task complexity result")
	}
	return Result{
		Category:      category,
		Probability:   probability,
		Confidence:    categoryAnswer.Confidence,
		Consequential: consequentialAnswer.Noul,
		Model:         response.Model,
		Usage:         response.Usage,
	}, nil
}

func validProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
