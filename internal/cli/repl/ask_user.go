package repl

func (m *replModel) clearAskUser() {
	if m.stream.handler != nil {
		m.stream.handler.SetAskUser(nil)
	}
	m.askUser.Clear()
}

func (m *replModel) appendResolvedAskUserSegment() {
	if !m.askUser.Completed || m.stream.handler == nil {
		return
	}
	m.stream.handler.SetAskUser(m.askUser.Card())
	m.clearAskUser()
}
