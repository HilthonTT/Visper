package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func (m model) HeaderUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n":
			if m.page != newRoomPage {
				return m.NewRoomSwitch()
			}
		case "j":
			if m.page != joinRoomPage {
				return m.JoinRoomSwitch()
			}
		case "f":
			if m.page != faqPage {
				return m.FaqSwitch()
			}
		case "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) HeaderView() string {
	bold := m.theme.TextAccent().Bold(true).Render
	accent := m.theme.TextAccent().Render
	base := m.theme.Base().Render
	cursor := m.theme.Base().Background(m.theme.Brand()).Render(" ")

	menu := bold("m") + base(" ☰")
	mark := bold("t") + cursor
	logo := bold("visper")
	newRoom := accent("n") + base(" new room")
	joinRoom := accent("j") + base(" join room")
	faq := accent("f") + base(" faq")

	switch m.page {
	case newRoomPage:
		newRoom = accent("n new room")
	case joinRoomPage:
		joinRoom = accent("j join room")
	case faqPage:
		faq = accent("f faq")
	}

	var tabs []string

	switch m.size {
	case small:
		tabs = []string{
			mark,
			newRoom,
			joinRoom,
		}
	case medium:
		tabs = []string{
			menu,
			logo,
			newRoom,
			joinRoom,
		}
	default:
		tabs = []string{
			menu,
			logo,
			newRoom,
			joinRoom,
			faq,
		}
	}

	var table = table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(m.renderer.NewStyle().Foreground(m.theme.Border())).
		Row(tabs...).
		Width(m.widthContent).
		StyleFunc(func(row, col int) lipgloss.Style {
			return m.theme.Base().
				Padding(0, 1).
				AlignHorizontal(lipgloss.Center)
		}).
		Render()

	return lipgloss.Place(
		m.widthContainer,
		lipgloss.Height(table),
		lipgloss.Center,
		lipgloss.Center,
		table,
	)
}
