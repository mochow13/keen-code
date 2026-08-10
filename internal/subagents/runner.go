package subagents

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

type ClientFactory func(*config.ResolvedConfig) (llm.LLMClient, error)
type ConfigResolver func(Profile) (*config.ResolvedConfig, error)
type RegistryFactory func(Profile, *tools.Registry) *tools.Registry

const defaultTimeoutSeconds = 1800

type Runner struct {
	WorkingDir       string
	Config           *config.ResolvedConfig
	GetProfiles      func() []Profile
	NewClient        ClientFactory
	GetRegistry      func() *tools.Registry
	ResolveConfig    ConfigResolver
	NewRegistry      RegistryFactory
	ProjectContext   func() string
	GetSkillsCatalog func() string
	Activity         chan<- ToolActivity
	runCounter       atomic.Uint64
}

type Result struct {
	Agent  string `json:"agent"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (r *Runner) RunDelegate(ctx context.Context, agent string, instanceIndex, instanceCount int, task string) (any, error) {
	return r.run(ctx, agent, activityAgentName(agent, instanceIndex, instanceCount), task)
}

func (r *Runner) Run(ctx context.Context, agent, task string) (Result, error) {
	return r.run(ctx, agent, strings.TrimSpace(agent), task)
}

func (r *Runner) run(ctx context.Context, agent, activityAgent, task string) (Result, error) {
	if r.GetProfiles == nil {
		err := fmt.Errorf("subagent profile provider not initialized")
		return failedResult(agent, "", err.Error(), err)
	}
	profile, ok := Find(r.GetProfiles(), strings.TrimSpace(agent))
	if !ok {
		err := fmt.Errorf("unknown subagent %q", agent)
		return failedResult(agent, "", "unknown subagent", err)
	}
	if strings.TrimSpace(task) == "" {
		err := fmt.Errorf("task is required")
		return failedResult(profile.Name, "", err.Error(), err)
	}

	cfg, err := r.resolvedConfig(profile)
	if err != nil {
		return failedResult(profile.Name, "", err.Error(), err)
	}
	if r.NewClient == nil {
		err := fmt.Errorf("subagent client factory not initialized")
		return failedResult(profile.Name, "", err.Error(), err)
	}
	client, err := r.NewClient(cfg)
	if err != nil {
		return failedResult(profile.Name, "", err.Error(), err)
	}
	registry, err := r.toolRegistry(profile)
	if err != nil {
		return failedResult(profile.Name, "", err.Error(), err)
	}

	childCtx, cancel := context.WithTimeout(ctx, profileTimeout(profile))
	defer cancel()

	events, err := client.StreamChat(childCtx, r.childMessages(profile, task), registry, llm.StreamOptions{OneShot: true})
	if err != nil {
		return failedResult(profile.Name, "", err.Error(), err)
	}
	runID := fmt.Sprintf("subagent-%d", r.runCounter.Add(1))
	text, err := collectResult(childCtx, events, activityAgent, runID, r.Activity)
	if err != nil {
		return failedResult(profile.Name, text, err.Error(), err)
	}
	return Result{Agent: profile.Name, Status: "completed", Result: strings.TrimSpace(text)}, nil
}

func (r *Runner) resolvedConfig(profile Profile) (*config.ResolvedConfig, error) {
	if profile.Provider != "" {
		if r.ResolveConfig == nil {
			return nil, fmt.Errorf("subagent provider resolver not initialized")
		}
		return r.ResolveConfig(profile)
	}
	if r.Config == nil {
		return nil, fmt.Errorf("LLM config not initialized")
	}
	return cloneConfig(r.Config), nil
}

func (r *Runner) toolRegistry(profile Profile) (*tools.Registry, error) {
	if r.GetRegistry == nil {
		return nil, fmt.Errorf("parent tool registry provider not initialized")
	}
	if r.NewRegistry == nil {
		return nil, fmt.Errorf("subagent tool registry factory not initialized")
	}
	registry := r.NewRegistry(profile, r.GetRegistry())
	if registry == nil {
		return nil, fmt.Errorf("subagent tool registry factory returned nil")
	}
	return registry, nil
}

func profileTimeout(profile Profile) time.Duration {
	seconds := profile.TimeoutSeconds
	if seconds <= 0 {
		seconds = defaultTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func activityAgentName(agent string, index, count int) string {
	agent = strings.TrimSpace(agent)
	if count <= 1 {
		return agent
	}
	return fmt.Sprintf("%s-%d", agent, index)
}

func failedResult(agent, partial, message string, err error) (Result, error) {
	return Result{Agent: agent, Status: "error", Result: partial, Error: message}, err
}

func cloneConfig(cfg *config.ResolvedConfig) *config.ResolvedConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	if cfg.Headers != nil {
		cloned.Headers = make(map[string]string, len(cfg.Headers))
		maps.Copy(cloned.Headers, cfg.Headers)
	}
	return &cloned
}
