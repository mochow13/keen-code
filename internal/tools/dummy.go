package tools

import (
	"context"
	"fmt"
	"time"
)

type DummyTool struct{}

func NewDummyTool() *DummyTool {
	return &DummyTool{}
}

func (t *DummyTool) Name() string {
	return "dummy_echo"
}

func (t *DummyTool) Description() string {
	return "Echoes back the input message with a timestamp. " +
		"Useful for testing tool calling functionality. " +
		"Supports an optional delay_ms parameter to simulate processing time."
}

func (t *DummyTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The message to echo back",
			},
			"delay_ms": map[string]any{
				"type":        "integer",
				"description": "Optional delay in milliseconds to simulate processing",
				"default":     0,
			},
		},
		"required": []string{"message"},
	}
}

func (t *DummyTool) Execute(ctx context.Context, input any) (any, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected map[string]any, got %T", input)
	}

	message, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message is required and must be a string")
	}

	delayMs := 0
	if d, ok := params["delay_ms"].(float64); ok {
		delayMs = int(d)
	}

	start := time.Now()

	if delayMs > 0 {
		select {
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	duration := time.Since(start)

	return map[string]any{
		"echo":      fmt.Sprintf("Echo: %s", message),
		"timestamp": start.Unix(),
		"duration":  duration.String(),
	}, nil
}
