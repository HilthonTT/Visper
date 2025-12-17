package theme

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	renderer *lipgloss.Renderer

	border     lipgloss.TerminalColor
	background lipgloss.TerminalColor
	highlight  lipgloss.TerminalColor
	brand      lipgloss.TerminalColor
	error      lipgloss.TerminalColor
	body       lipgloss.TerminalColor
	accent     lipgloss.TerminalColor

	base lipgloss.Style
}

func BasicTheme(renderer *lipgloss.Renderer, highlight *string) Theme {
	base := Theme{
		renderer: renderer,
	}

	base.background = lipgloss.AdaptiveColor{Dark: "#0A0E14", Light: "#F8FAFC"}
	base.border = lipgloss.AdaptiveColor{Dark: "#2D3748", Light: "#CBD5E0"}
	base.body = lipgloss.AdaptiveColor{Dark: "#94A3B8", Light: "#64748B"}
	base.accent = lipgloss.AdaptiveColor{Dark: "#F1F5F9", Light: "#0F172A"}
	base.brand = lipgloss.Color("#3B82F6") // Blue
	if highlight != nil {
		base.highlight = lipgloss.Color(*highlight)
	} else {
		base.highlight = base.brand
	}
	base.error = lipgloss.Color("#EF4444") // Red

	base.base = renderer.NewStyle().Foreground(base.body)

	return base
}

func (b Theme) Body() lipgloss.TerminalColor {
	return b.body
}

func (b Theme) Highlight() lipgloss.TerminalColor {
	return b.highlight
}

func (b Theme) Brand() lipgloss.TerminalColor {
	return b.brand
}

func (b Theme) Background() lipgloss.TerminalColor {
	return b.background
}

func (b Theme) Accent() lipgloss.TerminalColor {
	return b.accent
}

func (b Theme) Base() lipgloss.Style {
	return b.base.Copy()
}

func (b Theme) TextBody() lipgloss.Style {
	return b.Base().Foreground(b.body)
}

func (b Theme) TextAccent() lipgloss.Style {
	return b.Base().Foreground(b.accent)
}

func (b Theme) TextHighlight() lipgloss.Style {
	return b.Base().Foreground(b.highlight)
}

func (b Theme) TextBrand() lipgloss.Style {
	return b.Base().Foreground(b.brand)
}

func (b Theme) TextError() lipgloss.Style {
	return b.Base().Foreground(b.error)
}

func (b Theme) PanelError() lipgloss.Style {
	return b.Base().Background(b.error).Foreground(b.accent)
}

func (b Theme) Border() lipgloss.TerminalColor {
	return b.border
}
