package tui

import "charm.land/lipgloss/v2"

func (m model) ResizeView() string {
	return lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			m.LogoView(),
			"",
			m.theme.TextAccent().Render("needs a bigger window"),
			m.theme.TextSubtle().Render("at least 30 × 12"),
		),
	)
}
