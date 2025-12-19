package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type headerStyles struct {
	bold          func(...string) string
	accent        func(...string) string
	base          func(...string) string
	borderStyle   lipgloss.Style
	cellStyleFunc func(row, col int) lipgloss.Style
}

func (m model) HeaderUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.NewRoomPage):
			if m.page != newRoomPage {
				return m.NewRoomSwitch()
			}
		case key.Matches(msg, keys.JoinRoomPage):
			if m.page != joinRoomPage {
				return m.JoinRoomSwitch()
			}
		case key.Matches(msg, keys.FaqPage):
			if m.page != faqPage {
				return m.FaqSwitch()
			}
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) HeaderView() string {
	// Pre-compute styles (only done once per render)
	styles := m.getHeaderStyles()

	// Build tabs based on size
	tabs := m.buildHeaderTabs(styles)

	// Create and render table
	headerTable := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(styles.borderStyle).
		Row(tabs...).
		Width(m.widthContent).
		StyleFunc(styles.cellStyleFunc).
		Render()

	return lipgloss.Place(
		m.widthContainer,
		lipgloss.Height(headerTable),
		lipgloss.Center,
		lipgloss.Center,
		headerTable,
	)
}

func (m model) getHeaderStyles() headerStyles {
	baseStyle := m.theme.Base()

	return headerStyles{
		bold:   m.theme.TextAccent().Bold(true).Render,
		accent: m.theme.TextAccent().Render,
		base:   baseStyle.Render,
		borderStyle: m.renderer.NewStyle().
			Foreground(m.theme.Border()),
		cellStyleFunc: func(row, col int) lipgloss.Style {
			return baseStyle.
				Padding(0, 1).
				AlignHorizontal(lipgloss.Center)
		},
	}
}

func (m model) buildHeaderTabs(styles headerStyles) []string {
	// Build menu and logo
	menu := styles.bold("m") + styles.base(" ☰")

	cursorHighlight := m.theme.Base().
		Background(m.theme.Brand()).
		Render(" ")
	mark := styles.bold("t") + cursorHighlight
	logo := styles.bold("visper")

	// Build tab items with active state
	newRoom := m.buildTab("n", "new room", m.page == newRoomPage, styles)
	joinRoom := m.buildTab("j", "join room", m.page == joinRoomPage, styles)
	faq := m.buildTab("f", "faq", m.page == faqPage, styles)

	// Return tabs based on screen size
	switch m.size {
	case small:
		return []string{mark, newRoom, joinRoom}
	case medium:
		return []string{menu, logo, newRoom, joinRoom}
	default:
		return []string{menu, logo, newRoom, joinRoom, faq}
	}
}

func (m model) buildTab(key, label string, active bool, styles headerStyles) string {
	if active {
		return styles.accent(key + " " + label)
	}
	return styles.accent(key) + styles.base(" "+label)
}
