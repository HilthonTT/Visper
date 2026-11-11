package tui

import (
	"charm.land/lipgloss/v2"
)

type footerCommand struct {
	key   string
	value string
}

type footerState struct {
	commands []footerCommand
}

// globalCommands are the hints shown after the page's own ones.
var globalCommands = []footerCommand{
	{"tab", "next page"},
	{"m", "menu"},
	{"?", "help"},
}

func (m model) FooterView() string {
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

	badge := m.theme.TextSuccess().Render("●") + m.theme.TextSubtle().Render(" e2e encrypted")
	if m.size != large {
		badge = ""
	}

	global := globalCommands
	if m.page == joinRoomPage {
		// Letters go to the code field here.
		global = []footerCommand{{"tab", "next page"}}
	}
	cmds := append(append([]footerCommand{}, m.state.footer.commands...), global...)
	hints := m.keyHints(cmds, width-lipgloss.Width(badge)-2)

	return errorBanner + lipgloss.JoinVertical(
		lipgloss.Left,
		m.rule(width),
		spread(width, hints, badge),
	)
}
