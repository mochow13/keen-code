// Package liquid provides a decision evaluator for Liquid AI's d1 decision
// models. The Liquid API is SystemOne-compatible, so it reuses the TypeSafe
// evaluator with the Liquid base URL.
package liquid

import (
	"encoding/json"
	"fmt"

	"github.com/mochow13/keen-code/internal/decision"
	"github.com/mochow13/keen-code/internal/decision/typesafe"
)

const (
	ProviderID = "liquid"
	defaultURL = "https://api.liquid.ai/decisions"
)

type Factory struct{}

func (Factory) ID() string { return ProviderID }

func (Factory) New(rawConfig json.RawMessage) (decision.Evaluator, error) {
	var cfg typesafe.Config
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("decode Liquid AI decision config: %w", err)
		}
	}
	return typesafe.NewWithProvider(cfg, ProviderID, defaultURL)
}
