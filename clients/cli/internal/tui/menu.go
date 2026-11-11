package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The menu is a command palette drawn over the current page, and the help
// screen is an overlay listing every shortcut.

type menuState struct {
	open   bool
	cursor int
	help   bool
}

type menuItem struct {
	key   string
	label string
	desc  string
}

var menuItems = []menuItem{
	{"n", "New room", "start an encrypted room"},
	{"j", "Join room", "use a shared code"},
	{"f", "FAQ", "how Visper works"},
	{"s", "Settings", "pick your companion"},
	{"?", "Help", "all shortcuts"},
	{"q", "Quit", "leave Visper"},
}

func (m model) MenuSwitch() (model, tea.Cmd) {
	m.state.menu.open = true
	m.state.menu.cursor = 0
	for i, it := range menuItems {
		if (it.key == "n" && m.page == newRoomPage) || (it.key == "j" && m.page == joinRoomPage) ||
			(it.key == "f" && m.page == faqPage) || (it.key == "s" && m.page == settingsPage) {
			m.state.menu.cursor = i
		}
	}
	return m, nil
}

func (m model) MenuUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc", "m":
		m.state.menu.open = false
		return m, nil
	case "up", "k", "shift+tab":
		m.state.menu.cursor = (m.state.menu.cursor - 1 + len(menuItems)) % len(menuItems)
		return m, nil
	case "down", "tab":
		m.state.menu.cursor = (m.state.menu.cursor + 1) % len(menuItems)
		return m, nil
	case "enter", "space":
		return m.runMenuItem(menuItems[m.state.menu.cursor].key)
	}
	for _, it := range menuItems {
		if msg.String() == it.key {
			return m.runMenuItem(it.key)
		}
	}
	return m, nil
}

func (m model) runMenuItem(k string) (model, tea.Cmd) {
	m.state.menu.open = false
	switch k {
	case "n":
		return m.NewRoomSwitch()
	case "j":
		return m.JoinRoomSwitch()
	case "f":
		return m.FaqSwitch()
	case "s":
		return m.SettingsSwitch()
	case "?":
		m.state.menu.help = true
		return m, nil
	case "q":
		return m, tea.Sequence(m.cleanupCmd, tea.Quit)
	}
	return m, nil
}

func (m model) MenuView() string {
	width := min(44, m.viewportWidth-4)
	inner := width - 4

	var rows []string
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

	title := m.LogoView() + m.theme.TextSubtle().Render("  menu")
	hints := m.keyHints([]footerCommand{{"↑↓", "move"}, {"enter", "open"}, {"esc", "close"}}, inner)

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 1).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", strings.Join(rows, "\n"), "", hints))
}

type helpSection struct {
	title string
	keys  []footerCommand
}

var helpSections = []helpSection{
	{"Anywhere", []footerCommand{
		{"tab / ←→", "switch page"},
		{"n j f s", "new · join · faq · settings"},
		{"m", "menu"},
		{"?", "this help"},
		{"ctrl+c", "quit"},
	}},
	{"In a room", []footerCommand{
		{"enter", "send message"},
		{"pgup/pgdn", "scroll history"},
		{"ctrl+e", "edit / delete your messages"},
		{"ctrl+u", "share an image"},
		{"ctrl+a", "rewrite draft with AI"},
		{"ctrl+s", "search members"},
		{"ctrl+p", "QR invite"},
		{"ctrl+n", "new join code (owner)"},
		{"ctrl+w", "show / hide companion"},
		{"ctrl+b", "leave room"},
	}},
}

func (m model) HelpView() string {
	width := min(56, m.viewportWidth-4)
	inner := width - 4

	keyWidth := 0
	for _, s := range helpSections {
		for _, k := range s.keys {
			keyWidth = max(keyWidth, lipgloss.Width(k.key))
		}
	}

	var parts []string
	for i, s := range helpSections {
		if i > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, m.theme.TextSecondary().Bold(true).Render(s.title))
		for _, k := range s.keys {
			parts = append(parts,
				m.theme.Key().Width(keyWidth+2).Render(k.key)+m.theme.TextBody().Render(k.value))
		}
	}
	parts = append(parts, "", m.theme.TextSubtle().Render("press any key to close"))

	// Drop lines that don't fit on short terminals.
	maxLines := max(m.viewportHeight-6, 3)
	if len(parts) > maxLines {
		parts = parts[:maxLines]
	}

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 1).
		Width(width).
		Render(m.theme.Base().Width(inner).Render(strings.Join(parts, "\n")))
}
