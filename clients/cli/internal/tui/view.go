package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
)

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	switch {
	case m.size == undersized:
		return m.tooSmallView()
	case m.page == splashPage:
		return m.splashView()
	case m.page == chatPage:
		// The chat draws its own overlays. The toast sits over the message
		// column, just above the input box.
		return m.withToastAt(m.chatView(), m.chatLeftWidth()+m.chatCenterWidth()-1, chatInputHeight+chatStatusHeight)
	}

	header := m.headerView()
	footer := m.footerView()
	height := max(m.heightContainer-lipgloss.Height(header)-lipgloss.Height(footer), 1)
	body := m.theme.Base().
		Width(m.widthContainer).
		Height(height).
		MaxHeight(height).
		Render(m.pageBody(height))

	view := lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left, header, body, footer),
	)

	// Bottom-right of the centred layout, just above the status bar.
	right := (m.viewportWidth + m.widthContainer) / 2
	bottom := (m.viewportHeight-m.heightContainer)/2 + 3
	view = m.withToastAt(view, right, bottom)

	switch {
	case m.state.menu.help:
		view = m.overlay(m.helpView(), view)
	case m.state.menu.open:
		view = m.overlay(m.menuView(), view)
	case m.state.dialog.open:
		view = m.overlay(m.dialogView(), view)
	}
	return view
}

// pageBody renders the current tabbed page between the header and footer.
func (m model) pageBody(height int) string {
	switch m.page {
	case newRoomPage:
		return m.newRoomView(height)
	case joinRoomPage:
		return m.joinRoomView(height)
	case faqPage:
		return m.faqView(height)
	case settingsPage:
		return m.settingsView(height)
	}
	return ""
}

// pageContent centres a page's content in the body area.
func (m model) pageContent(height int, content string) string {
	return lipgloss.Place(m.widthContainer, height, lipgloss.Center, lipgloss.Center, content)
}

// overlay draws fg centred on top of bg, with a one-cell clear margin so the
// page behind doesn't butt up against its border.
func (m model) overlay(fg, bg string) string {
	fg = lipgloss.NewStyle().Padding(0, 1).Render(fg)
	x := max((m.viewportWidth-lipgloss.Width(fg))/2, 0)
	y := max((m.viewportHeight-lipgloss.Height(fg))/2, 0)
	return strfn.PlaceOverlay(x, y, fg, bg)
}

// tooSmallView replaces everything when the window is below the minimum size.
func (m model) tooSmallView() string {
	return lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			m.logoView(),
			"",
			m.theme.TextAccent().Render("needs a bigger window"),
			m.theme.TextSubtle().Render("at least 30 × 12"),
		),
	)
}
