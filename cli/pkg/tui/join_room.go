package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) JoinRoomSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(joinRoomPage)
	return m, nil
}

func (m model) JoinRoomView() string {
	return "join room view"
}

func (m model) JoinRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	return m, nil
}
