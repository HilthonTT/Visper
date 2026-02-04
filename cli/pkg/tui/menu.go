package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type menuState struct {
	lastPage page
}

func (m model) MenuSwitch() (model, tea.Cmd) {
	m.state.menu.lastPage = m.page
	m = m.SwitchPage(menuPage)

	// Reconnect notification WebSocket if we came from chat
	// (where it was disconnected)
	if m.state.notification.wsConn == nil && m.userID != nil && *m.userID != "" {
		ctx, cancel := context.WithCancel(context.Background())
		m.state.notification.wsCtx = ctx
		m.state.notification.wsCancel = cancel
		return m, m.connectNotificationWebSocket()
	}

	return m, nil
}

func (m model) MenuUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			switch m.state.menu.lastPage {
			case joinRoomPage:
				return m.JoinRoomSwitch()
			case newRoomPage:
				return m.NewRoomSwitch()
			case faqPage:
				return m.FaqSwitch()
			default:
				return m.JoinRoomSwitch()
			}
		}
	}

	return m, nil
}

func (m model) MenuView() string {
	base := m.theme.Base().Render
	bold := m.theme.TextAccent().Bold(true).Render

	menu :=
		table.New().
			Border(lipgloss.HiddenBorder()).
			Row(bold("n"), base("new room")).
			Row(bold("j"), base("join room")).
			Row(bold("f"), base("faq")).
			Row("").
			StyleFunc(func(row, col int) lipgloss.Style {
				return m.theme.Base().
					Padding(0, 1).
					AlignHorizontal(lipgloss.Left)
			})

	for _, cmd := range m.state.footer.commands {
		if cmd.key == "n" || cmd.key == "j" || cmd.key == "f" {
			continue
		}

		menu.Row(bold(cmd.key), base(cmd.value))
	}

	modal := m.theme.Base().
		Padding(1).
		Border(lipgloss.NormalBorder(), true, false).
		BorderForeground(m.theme.Border()).
		Render

	return lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			m.LogoView(),
			modal(menu.Render()),
			m.theme.TextAccent().
				Width(m.widthContent).
				Padding(0, 1).
				AlignHorizontal(lipgloss.Center).
				Render("press esc to close"),
		),
	)
}
