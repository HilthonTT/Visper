package theme

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	renderer *lipgloss.Renderer

	error  lipgloss.TerminalColor
	accent lipgloss.TerminalColor
	brand  lipgloss.TerminalColor

	base lipgloss.Style
}

func (b Theme) Base() lipgloss.Style {
	return b.base.Copy()
}

func (b Theme) TextAccent() lipgloss.Style {
	return b.Base().Foreground(b.accent)
}

func (b Theme) TextError() lipgloss.Style {
	return b.Base().Foreground(b.error)
}

func (b Theme) Brand() lipgloss.TerminalColor {
	return b.brand
}
