package repl

import "github.com/mochow13/keen-code/internal/agentcore"

func (m *replModel) consumeTurnMemory() *agentcore.TurnMemory {
	if m.turn == nil {
		return nil
	}
	memory := m.turn.Memory()
	m.turn = nil
	return memory
}

func (m *replModel) clearTurnMemory() {
	m.turn = nil
}

func (m *replModel) turnMemoryWorkingDir() string {
	if m.agentCore != nil && m.agentCore.WorkingDir() != "" {
		return m.agentCore.WorkingDir()
	}
	if m.ctx != nil {
		return m.ctx.workingDir
	}
	return ""
}
