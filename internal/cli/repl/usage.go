package repl

import (
	"errors"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mochow13/keen-code/internal/agentcore"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/usage"
)

func makeUsageRecord(provider, model string, tokenUsage *agentcore.TokenUsage) (usage.Record, bool) {
	if tokenUsage == nil {
		return usage.Record{}, false
	}
	return usage.NewRecord(provider, model,
		tokenUsage.InputTokens, tokenUsage.OutputTokens,
		tokenUsage.CacheReadTokens, tokenUsage.CacheWriteTokens, tokenUsage.ReasoningTokens)
}

func appendUsageRecord(record usage.Record) {
	store, err := usage.DefaultStore()
	if err != nil {
		log.Printf("failed to initialize usage ledger: %v", err)
		return
	}
	if err := store.Append(record); err != nil {
		log.Printf("failed to record token usage: %v", err)
	}
}

// compactUsageLedger rolls the global ledger up into per-day totals at most
// once per UTC calendar day. Startup calls it so compaction does not depend on
// the user opening /usage.
func compactUsageLedger() {
	store, err := usage.DefaultStore()
	if err != nil {
		log.Printf("failed to initialize usage ledger: %v", err)
		return
	}
	if err := store.MaybeCompact(time.Now()); err != nil && !errors.Is(err, usage.ErrLocked) {
		log.Printf("failed to compact usage ledger: %v", err)
	}
}

func (m *replModel) recordUsage(provider, model string, tokenUsage *agentcore.TokenUsage) {
	if record, ok := makeUsageRecord(provider, model, tokenUsage); ok {
		appendUsageRecord(record)
	}
}

func (m *replModel) recordCurrentUsage(tokenUsage *agentcore.TokenUsage) {
	if m.ctx == nil || m.ctx.cfg == nil {
		return
	}
	m.recordUsage(m.ctx.cfg.Provider, m.ctx.cfg.Model, tokenUsage)
}

func (m *replModel) startUsageView() {
	store, err := usage.DefaultStore()
	if err != nil {
		m.output.AddError("Failed to load usage: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return
	}
	compactUsageLedger()
	records, err := store.Load()
	if err != nil {
		m.output.AddError("Failed to load usage: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return
	}

	now := time.Now()
	summaries := make([]usage.Summary, len(usage.Ranges))
	for i, dateRange := range usage.Ranges {
		summaries[i] = usage.Summarize(records, dateRange.Since(now))
	}
	m.usageView = replwidgets.NewUsageView(summaries)
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *replModel) handleUsageViewKeyMsg(msg tea.KeyPressMsg) (replModel, tea.Cmd) {
	switch msg.String() {
	case keyLeft:
		m.usageView.PrevRange()
	case keyRight:
		m.usageView.NextRange()
	case keyEsc, keyCtrlC:
		m.usageView = nil
	default:
		return *m, nil
	}
	m.updateViewportContent()
	if m.usageView == nil {
		m.viewport.GotoBottom()
	}
	return *m, nil
}

func (m *replModel) drainSubagentUsage() {
	drainUsageRecords(m.subagentUsage)
}

func drainUsageRecords(records <-chan usage.Record) {
	for records != nil {
		select {
		case record := <-records:
			appendUsageRecord(record)
		default:
			return
		}
	}
}
