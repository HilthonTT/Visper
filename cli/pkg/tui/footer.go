package tui

import "github.com/charmbracelet/lipgloss"

type footerCommand struct {
	key   string
	value string
}

type footerState struct {
	commands []footerCommand
}

func (m model) FooterView() string {
	bold := m.theme.TextAccent().Bold(true).Render
	base := m.theme.Base().Render

	table := m.theme.Base().
		Width(m.widthContent).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		PaddingBottom(1).
		Align(lipgloss.Center)

	if m.size == small {
		return table.Render(bold("m") + base(" menu"))
	}

	// Add other commands
	commands := []string{}
	for _, cmd := range m.state.footer.commands {
		commands = append(commands, bold(" "+cmd.key+" ")+base(cmd.value+"  "))
	}

	lines := []string{}
	lines = append(lines, commands...)

	var content string
	if m.error != nil {
		hint := "esc"

		// Calculate maximum width for error message to ensure it fits
		maxErrorWidth := m.widthContent - lipgloss.Width(hint) - 6

		// Handle wrapping for long error messages
		errorMsg := m.error.message
		if lipgloss.Width(errorMsg) > maxErrorWidth {
			// Split into multiple lines
			errorMsg = wordWrap(errorMsg, maxErrorWidth)
		}

		msg := m.theme.PanelError().Padding(0, 1).Render(errorMsg)

		// Calculate remaining space after rendering the message
		space := max(m.widthContent-lipgloss.Width(msg)-lipgloss.Width(hint)-2, 0)

		height := lipgloss.Height(msg)

		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			msg,
			m.theme.PanelError().Width(space).Height(height).Render(),
			m.theme.PanelError().Bold(true).Padding(0, 1).Height(height).Render(hint),
		)
	} else {
		content = m.theme.Base().Faint(true).Render("end-to-end encrypted • anonymous • ephemeral")
	}

	footer := lipgloss.JoinVertical(
		lipgloss.Center,
		"",
		content,
		table.Render(
			lipgloss.JoinHorizontal(
				lipgloss.Center,
				lines...,
			),
		))

	// Add the region selector and the rest of the commands
	return lipgloss.Place(
		m.widthContainer,
		lipgloss.Height(footer),
		lipgloss.Center,
		lipgloss.Center,
		footer,
	)
}
