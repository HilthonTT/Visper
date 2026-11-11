package tui

import "charm.land/lipgloss/v2"

// The footer shows the error banner, if any, and the key hints of the
// tabbed pages.

type footerState struct {
	// hints are the current page's own hints, shown before the global ones.
	hints []keyHint
}

// globalHints are the hints shown after the page's own ones.
var globalHints = []keyHint{
	{"tab", "next page"},
	{"m", "menu"},
	{"?", "help"},
}

func (m model) setFooterHints(hints ...keyHint) model {
	m.state.footer.hints = hints
	return m
}

func (m model) footerView() string {
	width := m.widthContainer

	var errorBanner string
	if m.error != nil {
		text := m.theme.TextError().Width(width - 4).Render("✕ " + m.error.message)
		dismiss := m.theme.TextSubtle().Render("esc to dismiss")
		errorBanner = m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Error()).
			Padding(0, 1).
			Width(width).
			Render(text+"\n"+dismiss) + "\n"
	}

	var badge string
	if m.size == large {
		badge = m.theme.TextSuccess().Render("●") + m.theme.TextSubtle().Render(" e2e encrypted")
	}

	global := globalHints
	if m.page == joinRoomPage {
		// Letters go to the code field here.
		global = []keyHint{{"tab", "next page"}}
	}
	hints := m.keyHints(append(append([]keyHint{}, m.state.footer.hints...), global...), width-lipgloss.Width(badge)-2)

	return errorBanner + lipgloss.JoinVertical(
		lipgloss.Left,
		m.rule(width),
		spread(width, hints, badge),
	)
}
