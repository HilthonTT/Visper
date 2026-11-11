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
			m.theme.TextAccent().Render("your"),
			m.LogoView(),
			m.theme.TextAccent().Render("is too small"),
		),
	)
}
