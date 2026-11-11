package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The menu is a command palette drawn over the current page.

type menuState struct {
	open   bool
	cursor int
	// help is the shortcut overlay (see help.go), opened from the menu or "?".
	help bool
}

// menuItem is a palette entry; page is the page it opens, or noPage.
type menuItem struct {
	key   string
	label string
	desc  string
	page  page
}

// noPage marks a menu item that runs an action instead of opening a page.
const noPage page = -1

var menuItems = []menuItem{
	{"n", "New room", "start an encrypted room", newRoomPage},
	{"j", "Join room", "use a shared code", joinRoomPage},
	{"f", "FAQ", "how Visper works", faqPage},
	{"s", "Settings", "pick your companion", settingsPage},
	{"?", "Help", "all shortcuts", noPage},
	{"q", "Quit", "leave Visper", noPage},
}

// openMenu opens the palette with the current page highlighted.
func (m model) openMenu() (model, tea.Cmd) {
	m.state.menu.open = true
	m.state.menu.cursor = 0
	for i, it := range menuItems {
		if it.page == m.page {
			m.state.menu.cursor = i
		}
	}
	return m, nil
}

func (m model) menuUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	n := len(menuItems)
	switch msg.String() {
	case "esc", "m":
		m.state.menu.open = false
		return m, nil
	case "up", "k", "shift+tab":
		m.state.menu.cursor = (m.state.menu.cursor - 1 + n) % n
		return m, nil
	case "down", "tab":
		m.state.menu.cursor = (m.state.menu.cursor + 1) % n
		return m, nil
	case "enter", "space":
		return m.runMenuItem(menuItems[m.state.menu.cursor])
	}
	for _, it := range menuItems {
		if msg.String() == it.key {
			return m.runMenuItem(it)
		}
	}
	return m, nil
}

func (m model) runMenuItem(it menuItem) (model, tea.Cmd) {
	m.state.menu.open = false
	switch it.key {
	case "?":
		m.state.menu.help = true
		return m, nil
	case "q":
		return m, m.quit()
	}
	return m.gotoPage(it.page)
}

func (m model) menuView() string {
	width := min(44, m.viewportWidth-4)
	inner := width - 4

	rows := make([]string, 0, len(menuItems))
	for i, it := range menuItems {
		k := m.theme.Key().Width(2).Render(it.key)
		label := m.theme.TextAccent().Render(it.label)
		desc := m.theme.TextSubtle().Render(it.desc)
		line := spread(inner-2, k+" "+label, desc)
		if lipgloss.Width(line) > inner-2 {
			line = k + " " + label
		}
		if i == m.state.menu.cursor {
			rows = append(rows, m.theme.Base().Background(m.theme.Surface()).Width(inner).Render(
				m.theme.TextBrand().Bold(true).Render("▌")+" "+line,
			))
		} else {
			rows = append(rows, m.theme.Base().Width(inner).Render("  "+line))
		}
	}

	title := m.logoView() + m.theme.TextSubtle().Render("  menu")
	hints := m.keyHints([]keyHint{{"↑↓", "move"}, {"enter", "open"}, {"esc", "close"}}, inner)

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 1).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", strings.Join(rows, "\n"), "", hints))
}
