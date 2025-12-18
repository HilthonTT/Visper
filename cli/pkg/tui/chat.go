package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) ChatSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(chatPage)
	return m, nil
}

func (m model) ChatUpdate(msg tea.Msg) (model, tea.Cmd) {
	return m, nil
}

func (m model) ChatView() string {
	return "chat view"
}
