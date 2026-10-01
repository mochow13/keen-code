package repl

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/mochow13/keen-code/internal/agentcore"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
	"github.com/mochow13/keen-code/internal/config"

	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/usage"
)

func TestUsageViewKeysCycleAndClose(t *testing.T) {
	m := replModel{
		viewport:  viewport.New(viewport.WithWidth(80), viewport.WithHeight(24)),
		usageView: replwidgets.NewUsageView([]usage.Summary{{}, {}, {}, {}, {}, {}}),
	}

	updated, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyLeft})
	if cmd != nil || updated.usageView == nil || updated.usageView.RangeIndex() != 5 {
		t.Fatalf("left key should wrap to last range: view=%v cmd=%v", updated.usageView, cmd)
	}
	updated, cmd = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyRight})
	if cmd != nil || updated.usageView == nil || updated.usageView.RangeIndex() != 0 {
		t.Fatalf("right key should wrap to first range: view=%v cmd=%v", updated.usageView, cmd)
	}

	updated, cmd = updated.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil || updated.usageView == nil || updated.usageView.RangeIndex() != 0 {
		t.Fatalf("unrelated key should be swallowed: view=%v cmd=%v", updated.usageView, cmd)
	}
	updated, cmd = updated.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil || updated.usageView != nil {
		t.Fatalf("escape should close usage view: view=%v cmd=%v", updated.usageView, cmd)
	}

	updated.usageView = replwidgets.NewUsageView([]usage.Summary{{}, {}, {}, {}, {}, {}})
	updated, cmd = updated.handleKeyMsg(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'c'})
	if cmd != nil || updated.usageView != nil {
		t.Fatalf("ctrl+c should close usage view: view=%v cmd=%v", updated.usageView, cmd)
	}
}

func TestMakeUsageRecordSplitsCacheTokens(t *testing.T) {
	record, ok := makeUsageRecord("anthropic", "claude-sonnet-4-5", &agentcore.TokenUsage{
		InputTokens:      100,
		OutputTokens:     20,
		ReasoningTokens:  3,
		CacheReadTokens:  40,
		CacheWriteTokens: 5,
	})
	if !ok {
		t.Fatal("expected usage record")
	}
	if record.Provider != "anthropic" || record.Model != "claude-sonnet-4-5" || record.Input != 100 || record.Output != 20 || record.Reasoning != 3 || record.CacheRead != 40 || record.CacheWrite != 5 {
		t.Fatalf("unexpected usage record: %+v", record)
	}
	if record.TS.IsZero() || record.TS.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %v", record.TS)
	}
	if _, ok := makeUsageRecord("", "model", &agentcore.TokenUsage{}); ok {
		t.Fatal("expected incomplete provider/model to be ignored")
	}
	if _, ok := makeUsageRecord("provider", "model", nil); ok {
		t.Fatal("expected nil usage to be ignored")
	}
}

func TestUsageCaptureRecordsMainBtwAdversaryAndAutoCompaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := replModel{
		ctx: &replContext{
			cfg:       &config.ResolvedConfig{Provider: "primary", Model: "main-model"},
			globalCfg: &config.GlobalConfig{AdversaryProvider: "critic", AdversaryModel: "critic-model"},
		},
		stream: streamState{handler: replstream.NewStreamHandler(nil)},
	}

	m.handleLLMUsage(&agentcore.TokenUsage{InputTokens: 4})

	btwEvents := make(chan agentcore.StreamEvent)
	m.btw.streamHandler = replstream.NewStreamHandler(nil)
	m.btw.streamHandler.Start(btwEvents, "")
	updated, _, handled := m.handleBtwStreamMsg(btwUsageMsg{usage: &agentcore.TokenUsage{InputTokens: 3}})
	if !handled {
		t.Fatal("expected /btw usage event to be handled")
	}
	m = updated

	adversaryEvents := make(chan agentcore.StreamEvent)
	m.adversary.streamHandler = replstream.NewStreamHandler(nil)
	m.adversary.streamHandler.Start(adversaryEvents, "")
	updated, _, handled = m.handleAdversaryStreamMsg(adversaryUsageMsg{usage: &agentcore.TokenUsage{InputTokens: 2}})
	if !handled {
		t.Fatal("expected /adversary usage event to be handled")
	}
	m = updated

	m.handleAutoCompactionApplied(&agentcore.AutoCompactionEvent{Usage: &agentcore.TokenUsage{InputTokens: 1}})

	store, err := usage.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("expected one record per provider response, got %d: %+v", len(records), records)
	}
	wantProviders := []string{"primary/main-model", "primary/main-model", "critic/critic-model", "primary/main-model"}
	wantInputs := []int{4, 3, 2, 1}
	for i, record := range records {
		if got := record.Provider + "/" + record.Model; got != wantProviders[i] || record.Input != wantInputs[i] {
			t.Errorf("record %d = %s input %d, want %s input %d", i, got, record.Input, wantProviders[i], wantInputs[i])
		}
	}
}

func TestSubmitUsageCommandDoesNotAddMessageToViewport(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.submitInput("/usage", false)
	if cmd != nil {
		t.Fatalf("unexpected command: %v", cmd)
	}
	if updated.usageView == nil {
		t.Fatal("expected usage view to open")
	}
	if strings.Contains(updated.output.Join(), "/usage") {
		t.Fatalf("usage command should not appear in viewport output: %q", updated.output.Join())
	}
}

func TestCompactUsageLedgerRollsUpRecordsDaily(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := usage.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	year, month, dayNum := time.Now().UTC().AddDate(0, 0, -2).Date()
	day := time.Date(year, month, dayNum, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := store.Append(usage.Record{TS: day.Add(time.Duration(i) * time.Hour), Provider: "p", Model: "m", Input: 10}); err != nil {
			t.Fatal(err)
		}
	}

	compactUsageLedger()

	records, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Rollup || records[0].Input != 20 {
		t.Fatalf("expected a single merged rollup, got %+v", records)
	}

	// Same UTC day: throttled, so new raw records stay unrolled until tomorrow.
	if err := store.Append(usage.Record{TS: day.Add(3 * time.Hour), Provider: "p", Model: "m", Input: 5}); err != nil {
		t.Fatal(err)
	}
	compactUsageLedger()
	records, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("same-day compaction should be throttled, got %+v", records)
	}
}
