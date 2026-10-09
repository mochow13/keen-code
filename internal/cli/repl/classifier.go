package repl

import (
	"context"
	"fmt"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision"
	decisionliquid "github.com/mochow13/keen-code/internal/decision/liquid"
	"github.com/mochow13/keen-code/internal/decision/tasks/taskcomplexity"
	decisiontypesafe "github.com/mochow13/keen-code/internal/decision/typesafe"
)

// classificationManager owns the REPL state and lifecycle for decision tasks.
// Individual task implementations remain independent of the REPL.
type classificationManager struct {
	complexity         *taskcomplexity.Classifier
	complexityExchange []taskcomplexity.Message
}

func newClassificationManager(_ *config.GlobalConfig) (*classificationManager, error) {
	return &classificationManager{}, nil
}

func (m *classificationManager) Configure(global *config.GlobalConfig) error {
	if m == nil {
		return fmt.Errorf("classification manager is unavailable")
	}
	m.complexity = nil
	if global == nil {
		return fmt.Errorf("decision configuration is unavailable")
	}

	resolved, err := config.ResolveDecision(global)
	if err != nil {
		return err
	}
	registry, err := decision.Load(decisiontypesafe.Factory{}, decisionliquid.Factory{})
	if err != nil {
		return err
	}
	evaluator, err := registry.New(resolved.Provider, resolved.Config)
	if err != nil {
		return err
	}
	classifier, err := taskcomplexity.New(evaluator, resolved.Model)
	if err != nil {
		return err
	}
	m.complexity = classifier
	return nil
}

func (m *classificationManager) RecordUserMessage(content string) {
	if m == nil {
		return
	}
	m.complexityExchange = append(m.complexityExchange, taskcomplexity.Message{Role: "user", Content: content})
}

func (m *classificationManager) RecordAssistantMessage(content string) {
	if m == nil || content == "" {
		return
	}
	m.complexityExchange = append(m.complexityExchange, taskcomplexity.Message{Role: "assistant", Content: content})
}

func (m *classificationManager) Reset() {
	if m != nil {
		m.complexityExchange = nil
	}
}

func (m *classificationManager) ClassifyTask(ctx context.Context, cwd, gitBranch string) (taskcomplexity.Result, string, error) {
	if m == nil || m.complexity == nil {
		return taskcomplexity.Result{}, "", fmt.Errorf("task complexity classifier is unavailable")
	}
	exchange := append([]taskcomplexity.Message(nil), m.complexityExchange...)
	result, err := m.complexity.Classify(ctx, taskcomplexity.Input{
		Messages:  exchange,
		Cwd:       cwd,
		GitBranch: gitBranch,
	})
	return result, m.complexity.EvaluatorID(), err
}
