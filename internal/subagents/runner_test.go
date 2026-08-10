package subagents

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

type recordingClient struct {
	messages []llm.Message
	registry *tools.Registry
	options  []llm.StreamOptions
	timeout  time.Duration
	events   []llm.StreamEvent
}

func (c *recordingClient) StreamChat(ctx context.Context, messages []llm.Message, registry *tools.Registry, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	c.messages = llm.CloneMessages(messages)
	c.registry = registry
	c.options = append([]llm.StreamOptions(nil), opts...)
	if deadline, ok := ctx.Deadline(); ok {
		c.timeout = time.Until(deadline)
	}
	events := c.events
	if len(events) == 0 {
		events = []llm.StreamEvent{
			{Type: llm.StreamEventTypeChunk, Content: "summary"},
			{Type: llm.StreamEventTypeDone},
		}
	}
	ch := make(chan llm.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (c *recordingClient) Reset() {}

type namedTool struct{ name string }

func (t namedTool) Name() string                              { return t.name }
func (t namedTool) Description() string                       { return t.name }
func (t namedTool) InputSchema() map[string]any               { return map[string]any{"type": "object"} }
func (t namedTool) Execute(context.Context, any) (any, error) { return "ok", nil }

func staticProfiles(profiles ...Profile) func() []Profile {
	return func() []Profile { return profiles }
}

func staticRegistry(registry *tools.Registry) func() *tools.Registry {
	return func() *tools.Registry { return registry }
}

func namedRegistryFactory(profile Profile, parent *tools.Registry) *tools.Registry {
	child := tools.NewRegistry()
	for _, name := range permissionToolNames(profile, registryNames(parent)) {
		_ = child.Register(namedTool{name: name})
	}
	return child
}

func TestRunnerUsesInheritedRegistryAndProfilePrompt(t *testing.T) {
	parentRegistry := tools.NewRegistry()
	for _, name := range []string{"read_file", "glob", "grep", "write_file", "delegate_task"} {
		if err := parentRegistry.Register(namedTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	client := &recordingClient{}
	runner := &Runner{
		WorkingDir: "/repo",
		Config:     &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{
			Name:           "explorer",
			Description:    "Explore code",
			Permissions:    []string{"read"},
			PermissionsSet: true,
			Instructions:   "Explore only what was asked.",
		}),
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: staticRegistry(parentRegistry),
		NewRegistry: namedRegistryFactory,
	}

	result, err := runner.Run(context.Background(), "explorer", "Inspect internal/subagents")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != "completed" || result.Result != "summary" {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, name := range []string{"read_file", "glob", "grep"} {
		if _, ok := client.registry.Get(name); !ok {
			t.Fatalf("expected child registry to contain %s", name)
		}
	}
	for _, name := range []string{"write_file", "delegate_task"} {
		if _, ok := client.registry.Get(name); ok {
			t.Fatalf("expected child registry to exclude %s", name)
		}
	}
	if len(client.messages) != 2 {
		t.Fatalf("expected 2 child messages, got %d", len(client.messages))
	}
	if got := client.messages[0].Content; !containsAll(got, []string{"Explore only what was asked.", "Working directory: /repo"}) {
		t.Fatalf("system prompt missing expected content: %s", got)
	}
	if got := client.messages[1].Content; !containsAll(got, []string{"Delegated task:", "Inspect internal/subagents"}) {
		t.Fatalf("user task missing expected content: %s", got)
	}
	if len(client.options) != 1 || !client.options[0].OneShot {
		t.Fatalf("expected subagent stream to be one-shot, got %#v", client.options)
	}
}

func TestRunnerInheritsCurrentParentRegistry(t *testing.T) {
	fullRegistry := tools.NewRegistry()
	for _, name := range []string{"read_file", "write_file", "edit_file", "bash", "delegate_task"} {
		if err := fullRegistry.Register(namedTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	currentRegistry := fullRegistry.Without("write_file", "edit_file")
	client := &recordingClient{}
	factoryCalls := 0
	runner := &Runner{
		WorkingDir:  "/repo",
		Config:      &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{Name: "worker", Description: "Worker"}),
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: func() *tools.Registry {
			return currentRegistry
		},
		NewRegistry: func(profile Profile, parent *tools.Registry) *tools.Registry {
			factoryCalls++
			child := tools.NewRegistry()
			for _, name := range permissionToolNames(profile, registryNames(parent)) {
				_ = child.Register(namedTool{name: name})
			}
			return child
		},
	}

	if _, err := runner.Run(context.Background(), "worker", "Inspect the change"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if factoryCalls != 1 {
		t.Fatalf("expected one registry factory call, got %d", factoryCalls)
	}
	for _, name := range []string{"read_file", "bash"} {
		if _, ok := client.registry.Get(name); !ok {
			t.Fatalf("expected inherited child registry to contain %s", name)
		}
	}
	for _, name := range []string{"write_file", "edit_file", "delegate_task"} {
		if _, ok := client.registry.Get(name); ok {
			t.Fatalf("expected inherited child registry to exclude %s", name)
		}
	}
}

func TestToolFactoryUsesSuppliedParentRegistryForInheritance(t *testing.T) {
	parent := tools.NewRegistry()
	for _, name := range []string{"read_file", "bash", "delegate_task"} {
		if err := parent.Register(namedTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	factory := ToolFactory{Guard: filesystem.NewGuard(t.TempDir(), filesystem.NewGitAwareness())}
	registry := factory.Registry(Profile{}, parent)
	for _, name := range []string{"read_file", "bash"} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("expected factory registry to contain %s", name)
		}
	}
	for _, name := range []string{"write_file", "edit_file", "delegate_task"} {
		if _, ok := registry.Get(name); ok {
			t.Fatalf("expected factory registry to exclude %s", name)
		}
	}
}

func TestForwardActivityWaitsForCapacity(t *testing.T) {
	sink := make(chan ToolActivity, 1)
	sink <- ToolActivity{RunID: "existing"}
	forwarded := make(chan struct{})
	go func() {
		forwardActivity(context.Background(), sink, ToolActivity{RunID: "next"})
		close(forwarded)
	}()

	select {
	case <-forwarded:
		t.Fatal("forwardActivity returned while the sink was full")
	case <-time.After(20 * time.Millisecond):
	}
	<-sink
	select {
	case <-forwarded:
	case <-time.After(time.Second):
		t.Fatal("forwardActivity did not continue after capacity became available")
	}
	if activity := <-sink; activity.RunID != "next" {
		t.Fatalf("expected queued activity to be delivered, got %+v", activity)
	}
}

func TestRunnerResolvesProfileConfigAndForwardsSanitizedActivity(t *testing.T) {
	client := &recordingClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeReasoningChunk, Content: "hidden reasoning"},
		{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}}},
		{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "bash", Output: map[string]any{"stdout": "secret body"}, Duration: time.Second}},
		{Type: llm.StreamEventTypeChunk, Content: "private result"},
		{Type: llm.StreamEventTypeDone},
	}}
	activity := make(chan ToolActivity, 2)
	var resolved Profile
	runner := &Runner{
		WorkingDir: "/repo",
		Config:     &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "parent", ThinkingEffort: "high", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{
			Name: "worker", Description: "Worker", Provider: config.ProviderAnthropic, Model: "child", ThinkingEffort: "low",
		}),
		ResolveConfig: func(profile Profile) (*config.ResolvedConfig, error) {
			resolved = profile
			return &config.ResolvedConfig{Provider: profile.Provider, Model: profile.Model, ThinkingEffort: profile.ThinkingEffort, APIKey: "key"}, nil
		},
		NewClient: func(cfg *config.ResolvedConfig) (llm.LLMClient, error) {
			if cfg.Provider != config.ProviderAnthropic || cfg.Model != "child" || cfg.ThinkingEffort != "low" {
				t.Fatalf("unexpected child config: %+v", cfg)
			}
			return client, nil
		},
		GetRegistry: staticRegistry(tools.NewRegistry()),
		NewRegistry: namedRegistryFactory,
		Activity:    activity,
	}
	result, err := runner.Run(context.Background(), "worker", "Implement change")
	if err != nil || result.Result != "private result" {
		t.Fatalf("unexpected result=%+v err=%v", result, err)
	}
	if resolved.Provider != config.ProviderAnthropic || resolved.Model != "child" {
		t.Fatalf("resolver received wrong profile: %+v", resolved)
	}
	start := <-activity
	end := <-activity
	if start.Event.Type != llm.StreamEventTypeToolStart || end.Event.Type != llm.StreamEventTypeToolEnd {
		t.Fatalf("unexpected activity events: %+v %+v", start, end)
	}
	if start.Agent != "worker" || start.RunID == "" || start.CallID == "" || start.RunID != end.RunID || start.CallID != end.CallID {
		t.Fatalf("activity identity mismatch: %+v %+v", start, end)
	}
	if end.Event.ToolCall.Output != nil {
		t.Fatalf("tool result body leaked: %+v", end.Event.ToolCall.Output)
	}
}

func TestRunnerIndexesDelegatedAgentActivity(t *testing.T) {
	client := &recordingClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "README.md"}}},
		{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file"}},
		{Type: llm.StreamEventTypeDone},
	}}
	activity := make(chan ToolActivity, 2)
	runner := &Runner{
		WorkingDir:  "/repo",
		Config:      &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{Name: "explorer", Description: "Explore"}),
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: staticRegistry(tools.NewRegistry()),
		NewRegistry: namedRegistryFactory,
		Activity:    activity,
	}

	if _, err := runner.RunDelegate(context.Background(), "explorer", 2, 2, "Inspect files"); err != nil {
		t.Fatalf("RunDelegate returned error: %v", err)
	}
	for range 2 {
		if event := <-activity; event.Agent != "explorer-2" {
			t.Fatalf("expected indexed activity agent, got %+v", event)
		}
	}
}

func TestRunnerOmitsIndexForSingleAgentInstance(t *testing.T) {
	client := &recordingClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "README.md"}}},
		{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file"}},
		{Type: llm.StreamEventTypeDone},
	}}
	activity := make(chan ToolActivity, 2)
	runner := &Runner{
		WorkingDir:  "/repo",
		Config:      &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{Name: "explorer", Description: "Explore"}),
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: staticRegistry(tools.NewRegistry()),
		NewRegistry: namedRegistryFactory,
		Activity:    activity,
	}

	if _, err := runner.RunDelegate(context.Background(), "explorer", 1, 1, "Inspect files"); err != nil {
		t.Fatalf("RunDelegate returned error: %v", err)
	}
	for range 2 {
		if event := <-activity; event.Agent != "explorer" {
			t.Fatalf("expected unindexed activity agent, got %+v", event)
		}
	}
}

func TestRunnerUsesLiveProfileProvider(t *testing.T) {
	client := &recordingClient{}
	profiles := []Profile{{Name: "old", Description: "Old", Instructions: "Old prompt."}}
	runner := &Runner{
		WorkingDir: "/repo",
		Config:     &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
		GetProfiles: func() []Profile {
			return profiles
		},
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: staticRegistry(tools.NewRegistry()),
		NewRegistry: namedRegistryFactory,
	}

	profiles = []Profile{{Name: "explorer", Description: "Explore", Instructions: "Fresh prompt."}}
	result, err := runner.Run(context.Background(), "explorer", "Inspect files")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := client.messages[0].Content; !strings.Contains(got, "Fresh prompt.") {
		t.Fatalf("expected prompt from live profiles, got %s", got)
	}
}

func TestChildPromptOrdersProfileBeforeSecurityProjectSkillsAndBoundary(t *testing.T) {
	runner := &Runner{
		WorkingDir:       "/repo",
		ProjectContext:   func() string { return "# Project Instructions\n\nRun race tests." },
		GetSkillsCatalog: func() string { return "## Available Skills\n\n- review" },
	}
	prompt := runner.childPrompt(Profile{Instructions: "Return a typed handoff."})
	parts := []string{
		"# Profile Instructions", "Mandatory Security Instructions", "Working directory: /repo", "Run race tests.",
		"Available Skills", "Never invoke, request, or simulate nested subagents",
	}
	lastIndex := -1
	for _, part := range parts {
		index := strings.Index(prompt, part)
		if index == -1 {
			t.Fatalf("child prompt missing %q: %s", part, prompt)
		}
		if index < lastIndex {
			t.Fatalf("child prompt places %q out of order: %s", part, prompt)
		}
		lastIndex = index
	}
	for _, excluded := range []string{"# Memory", "Available Subagents", "parent conversation history"} {
		if strings.Contains(prompt, excluded) {
			t.Fatalf("child prompt unexpectedly includes %q", excluded)
		}
	}
}

func TestRunnerReturnsErrorsForInvalidInputs(t *testing.T) {
	runner := &Runner{
		GetProfiles: staticProfiles(Profile{Name: "explorer", Description: "Explore", Instructions: "Prompt."}),
		Config:      &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
	}
	if result, err := runner.Run(context.Background(), "missing", "Task"); err == nil || result.Status != "error" {
		t.Fatalf("expected unknown subagent error, got result=%+v err=%v", result, err)
	}
	if result, err := runner.Run(context.Background(), "explorer", ""); err == nil || result.Status != "error" {
		t.Fatalf("expected missing task error, got result=%+v err=%v", result, err)
	}

	runner.Config = nil
	if result, err := runner.Run(context.Background(), "explorer", "Task"); err == nil || result.Status != "error" {
		t.Fatalf("expected missing config error, got result=%+v err=%v", result, err)
	}
}

func TestRunnerAppliesProfileTimeout(t *testing.T) {
	newRunner := func(client *recordingClient, profileTimeout int) *Runner {
		return &Runner{
			WorkingDir:  "/repo",
			Config:      &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "model", APIKey: "key"},
			GetProfiles: staticProfiles(Profile{Name: "explorer", Description: "Explore", Instructions: "Prompt.", TimeoutSeconds: profileTimeout}),
			NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
			GetRegistry: staticRegistry(tools.NewRegistry()),
			NewRegistry: namedRegistryFactory,
		}
	}

	client := &recordingClient{}
	if _, err := newRunner(client, 30).Run(context.Background(), "explorer", "Task"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if client.timeout <= 29*time.Second || client.timeout > 30*time.Second {
		t.Fatalf("expected profile timeout near 30s, got %v", client.timeout)
	}

	client = &recordingClient{}
	if _, err := newRunner(client, 0).Run(context.Background(), "explorer", "Task"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := time.Duration(defaultTimeoutSeconds) * time.Second
	if client.timeout <= want-time.Second || client.timeout > want {
		t.Fatalf("expected default timeout near %v, got %v", want, client.timeout)
	}
}

func TestCollectResultReturnsPartialTextOnError(t *testing.T) {
	events := make(chan llm.StreamEvent, 2)
	events <- llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "partial"}
	events <- llm.StreamEvent{Type: llm.StreamEventTypeIncomplete}
	close(events)

	text, err := collectResult(context.Background(), events, "", "", nil)
	if err == nil {
		t.Fatal("expected incomplete stream error")
	}
	if text != "partial" {
		t.Fatalf("expected partial text, got %q", text)
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
