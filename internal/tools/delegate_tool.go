package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type SubagentRunner interface {
	RunDelegate(ctx context.Context, agent string, instanceIndex, instanceCount int, task string) (any, error)
}

const maxDelegateTasks = 10

type DelegateTool struct {
	runner SubagentRunner
}

type delegateInput struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

type delegateBatchInput struct {
	Tasks []delegateInput `json:"tasks"`
}

type delegateResult struct {
	Agent  string `json:"agent"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func NewDelegateTool(runner SubagentRunner) *DelegateTool {
	return &DelegateTool{runner: runner}
}

func (t *DelegateTool) Name() string {
	return "delegate_task"
}

func (t *DelegateTool) Description() string {
	return "Delegate up to 10 bounded tasks to named subagents and run them in parallel. Provide clear instructions and relevant paths for each task. Use a single-item tasks array when only one delegation is needed."
}

func (t *DelegateTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"tasks"},
		"properties": map[string]any{
			"tasks": map[string]any{
				"type":        "array",
				"description": "One to 10 independent tasks to run in parallel.",
				"minItems":    1,
				"maxItems":    maxDelegateTasks,
				"items": map[string]any{
					"type":     "object",
					"required": []string{"agent", "task"},
					"properties": map[string]any{
						"agent": map[string]any{
							"type":        "string",
							"description": "Name of the subagent profile to run.",
						},
						"task": map[string]any{
							"type":        "string",
							"description": "Bounded task for the subagent. Include relevant directories or file paths when possible.",
						},
					},
				},
			},
		},
	}
}

func (t *DelegateTool) ValidateInput(_ context.Context, input any) error {
	_, err := parseDelegateInput(input)
	return err
}

func (t *DelegateTool) Execute(ctx context.Context, input any) (any, error) {
	if t.runner == nil {
		return nil, fmt.Errorf("subagent runner not configured")
	}
	parsed, err := parseDelegateInput(input)
	if err != nil {
		return nil, err
	}

	results := make([]delegateResult, len(parsed.Tasks))
	counts := make(map[string]int, len(parsed.Tasks))
	for _, task := range parsed.Tasks {
		counts[task.Agent]++
	}
	indexes := make(map[string]int, len(counts))
	var wg sync.WaitGroup
	for i, task := range parsed.Tasks {
		indexes[task.Agent]++
		instanceIndex := indexes[task.Agent]
		instanceCount := counts[task.Agent]
		wg.Go(func() {
			result, runErr := t.runner.RunDelegate(ctx, task.Agent, instanceIndex, instanceCount, task.Task)
			results[i] = delegateResult{Agent: task.Agent, Result: result}
			if runErr != nil {
				results[i].Error = runErr.Error()
			}
		})
	}
	wg.Wait()
	failed := 0
	completedByAgent := make(map[string]int)
	failedByAgent := make(map[string]int)
	for _, result := range results {
		if result.Error != "" {
			failed++
			failedByAgent[result.Agent]++
			continue
		}
		completedByAgent[result.Agent]++
	}
	return map[string]any{
		"results":            results,
		"completed":          len(results) - failed,
		"failed":             failed,
		"completed_by_agent": completedByAgent,
		"failed_by_agent":    failedByAgent,
	}, nil
}

func parseDelegateInput(input any) (delegateBatchInput, error) {
	var parsed delegateBatchInput
	params, ok := input.(map[string]any)
	if !ok {
		return parsed, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	if _, exists := params["tasks"]; !exists {
		return parsed, missingRequiredParameter("delegate_task", "tasks", `{"tasks":[{"agent":"<subagent profile>","task":"<bounded task with relevant paths>"}]}`, "Provide between 1 and 10 independent tasks")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return parsed, err
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return parsed, err
	}
	if len(parsed.Tasks) == 0 {
		return parsed, fmt.Errorf("invalid input: tasks must contain at least one task")
	}
	if len(parsed.Tasks) > maxDelegateTasks {
		return parsed, fmt.Errorf("invalid input: at most %d tasks can be delegated at once, got %d", maxDelegateTasks, len(parsed.Tasks))
	}
	for i, task := range parsed.Tasks {
		if task.Agent == "" {
			return parsed, fmt.Errorf("invalid input: tasks[%d].agent must be a non-empty string", i)
		}
		if task.Task == "" {
			return parsed, fmt.Errorf("invalid input: tasks[%d].task must be a non-empty string", i)
		}
	}
	return parsed, nil
}
