// Package theme defines the colours and base styles of the TUI.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	border    color.Color
	highlight color.Color
	brand     color.Color
	error     color.Color
	body      color.Color
	accent    color.Color

	borderStyle lipgloss.Border

	base lipgloss.Style
}

// BasicTheme returns the default theme for a terminal with a dark (isDark)
// or light background. highlight optionally overrides the highlight colour.
func BasicTheme(isDark bool, highlight *string) Theme {
	lightDark := lipgloss.LightDark(isDark)
	base := Theme{}

	base.border = lightDark(lipgloss.Color("#CBD5E0"), lipgloss.Color("#2D3748"))
	base.body = lightDark(lipgloss.Color("#64748B"), lipgloss.Color("#94A3B8"))
	base.accent = lightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#F1F5F9"))
	base.brand = lipgloss.Color("#3B82F6") // Blue
	if highlight != nil {
		base.highlight = lipgloss.Color(*highlight)
	} else {
		base.highlight = base.brand
	}
	base.error = lipgloss.Color("#EF4444") // Red

	base.borderStyle = generateBorder()
	base.base = lipgloss.NewStyle().Foreground(base.body)

	return base
}

func (b Theme) Body() color.Color {
	return b.body
}

func (b Theme) Highlight() color.Color {
	return b.highlight
}

func (b Theme) Brand() color.Color {
	return b.brand
}

func (b Theme) Accent() color.Color {
	return b.accent
}

func (b Theme) Base() lipgloss.Style {
	return b.base
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

func (b Theme) Border() color.Color {
	return b.border
}

func (b Theme) Modal() lipgloss.Style {
	return b.Base().
		Border(b.borderStyle).
		BorderForeground(b.border).
		Foreground(b.accent).
		Align(lipgloss.Center)
}

func generateBorder() lipgloss.Border {
	return lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
	}
}
