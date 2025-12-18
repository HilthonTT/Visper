package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) NewRoomSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(newRoomPage)

	return m, nil
}

func (m model) NewRoomView() string {
	return "new room view"
}

func (m model) NewRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	return m, nil
}
