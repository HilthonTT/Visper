package tui

import (
	"errors"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type tab struct {
	page  page
	key   string
	label string
}

var tabs = []tab{
	{newRoomPage, "n", "New room"},
	{joinRoomPage, "j", "Join room"},
	{faqPage, "f", "FAQ"},
	{settingsPage, "s", "Settings"},
}

func (m model) HeaderUpdate(msg tea.Msg) (model, tea.Cmd) {
	if m.page == chatPage || m.page == splashPage {
		return m, nil
	}

	msg2, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	// tab / shift+tab cycle the pages everywhere, including the join page.
	switch msg2.String() {
	case "tab":
		return m.navigateToPage(1)
	case "shift+tab":
		return m.navigateToPage(-1)
	}

	// The join page has a focused text input: letters and arrows belong to
	// it, not to the page shortcuts.
	if m.page == joinRoomPage && (len(msg2.Text) > 0 || msg2.Code == tea.KeyLeft || msg2.Code == tea.KeyRight) {
		return m, nil
	}

	switch {
	case key.Matches(msg2, keys.Left):
		return m.navigateToPage(-1)
	case key.Matches(msg2, keys.Right):
		return m.navigateToPage(1)
	case key.Matches(msg2, keys.NewRoomPage):
		if m.page != newRoomPage {
			return m.NewRoomSwitch()
		}
	case key.Matches(msg2, keys.JoinRoomPage):
		if m.page != joinRoomPage {
			return m.JoinRoomSwitch()
		}
	case key.Matches(msg2, keys.FaqPage):
		if m.page != faqPage {
			return m.FaqSwitch()
		}
	case key.Matches(msg2, keys.SettingsPage):
		if m.page != settingsPage {
			return m.SettingsSwitch()
		}
	}

	return m, nil
}

func (m model) HeaderView() string {
	width := m.widthContainer

	var items []string
	for _, t := range tabs {
		if !m.tabVisible(t.page) {
			continue
		}
		items = append(items, m.renderTab(t, m.page == t.page))
	}
	left := m.LogoView() + "  " + lipgloss.JoinHorizontal(lipgloss.Top, items...)

	var right string
	switch m.size {
	case large:
		if m.state.notification.wsConn != nil {
			right = m.theme.TextSuccess().Render("●") + m.theme.TextBody().Render(" online")
		} else {
			right = m.theme.TextSubtle().Render("○ offline")
		}
	case small:
		right = m.theme.Key().Render("m") + m.theme.TextBody().Render(" menu")
	}

	line := spread(width, left, right)
	if lipgloss.Width(line) > width {
		line = left
	}

	return lipgloss.JoinVertical(lipgloss.Left, "", line, m.rule(width))
}

// tabVisible reports whether a page gets a tab at the current size. On narrow
// terminals only the active tab is shown; the rest stay one key away.
func (m model) tabVisible(p page) bool {
	switch m.size {
	case small:
		return p == m.page
	case medium:
		return p == m.page || p == newRoomPage || p == joinRoomPage
	default:
		return true
	}
}

func (m model) renderTab(t tab, active bool) string {
	if active {
		return m.theme.Pill(m.theme.Brand(), m.theme.OnBrand()).Render(t.label)
	}
	return m.theme.Base().Padding(0, 1).Render(
		m.theme.TextSubtle().Render(t.key) + " " + m.theme.TextBody().Render(t.label),
	)
}

func (m model) getNavigablePages() []page {
	pages := make([]page, len(tabs))
	for i, t := range tabs {
		pages[i] = t.page
	}
	return pages
}

func (m model) getCurrentPageIndex() (int, error) {
	pages := m.getNavigablePages()
	for i, p := range pages {
		if p == m.page {
			return i, nil
		}
	}
	return -1, errors.New("page not found")
}

func (m model) navigateToPage(direction int) (model, tea.Cmd) {
	currentIndex, err := m.getCurrentPageIndex()
	if err != nil {
		return m, nil
	}

	pages := m.getNavigablePages()
	newIndex := (currentIndex + direction + len(pages)) % len(pages)
	return m.switchTo(pages[newIndex])
}

// switchTo opens a top-level page.
func (m model) switchTo(p page) (model, tea.Cmd) {
	switch p {
	case newRoomPage:
		return m.NewRoomSwitch()
	case joinRoomPage:
		return m.JoinRoomSwitch()
	case faqPage:
		return m.FaqSwitch()
	case settingsPage:
		return m.SettingsSwitch()
	default:
		return m, nil
	}
}
