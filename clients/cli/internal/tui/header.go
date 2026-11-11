package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The header is the logo and the page tabs above the tabbed pages.

type tab struct {
	page  page
	key   key.Binding
	label string
}

var tabs = []tab{
	{newRoomPage, keys.NewRoomPage, "New room"},
	{joinRoomPage, keys.JoinRoomPage, "Join room"},
	{faqPage, keys.FaqPage, "FAQ"},
	{settingsPage, keys.SettingsPage, "Settings"},
}

// headerUpdate handles the page navigation keys of the tabbed pages.
func (m model) headerUpdate(msg tea.Msg) (model, tea.Cmd) {
	if m.page == chatPage || m.page == splashPage {
		return m, nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	// tab / shift+tab cycle the pages everywhere, including the join page.
	switch {
	case key.Matches(k, keys.NextTab):
		return m.cyclePage(1)
	case key.Matches(k, keys.PrevTab):
		return m.cyclePage(-1)
	}

	// The join page has a focused text input: letters and arrows belong to
	// it, not to the page shortcuts.
	if m.page == joinRoomPage && (len(k.Text) > 0 || k.Code == tea.KeyLeft || k.Code == tea.KeyRight) {
		return m, nil
	}

	switch {
	case key.Matches(k, keys.PrevPage):
		return m.cyclePage(-1)
	case key.Matches(k, keys.NextPage):
		return m.cyclePage(1)
	}
	for _, t := range tabs {
		if key.Matches(k, t.key) && m.page != t.page {
			return m.gotoPage(t.page)
		}
	}
	return m, nil
}

// cyclePage moves direction tabs from the current page, wrapping around.
func (m model) cyclePage(direction int) (model, tea.Cmd) {
	i := slices.IndexFunc(tabs, func(t tab) bool { return t.page == m.page })
	if i < 0 {
		return m, nil
	}
	next := (i + direction + len(tabs)) % len(tabs)
	return m.gotoPage(tabs[next].page)
}

func (m model) headerView() string {
	width := m.widthContainer

	var items []string
	for _, t := range tabs {
		if m.tabVisible(t.page) {
			items = append(items, m.tabView(t, m.page == t.page))
		}
	}
	left := m.logoView() + "  " + lipgloss.JoinHorizontal(lipgloss.Top, items...)

	var right string
	switch m.size {
	case large:
		if m.state.notification.conn != nil {
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

func (m model) tabView(t tab, active bool) string {
	if active {
		return m.theme.Pill(m.theme.Brand(), m.theme.OnBrand()).Render(t.label)
	}
	return m.theme.Base().Padding(0, 1).Render(
		m.theme.TextSubtle().Render(t.key.Help().Key) + " " + m.theme.TextBody().Render(t.label),
	)
}
