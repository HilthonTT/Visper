// Package theme defines the colours and base styles of the TUI.
package theme

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Tone is the semantic colour of a dialog, toast or badge.
type Tone int

const (
	ToneInfo Tone = iota
	ToneSuccess
	ToneWarning
	ToneDanger
)

type Theme struct {
	isDark bool

	border    color.Color
	highlight color.Color
	brand     color.Color
	secondary color.Color
	error     color.Color
	warning   color.Color
	success   color.Color
	body      color.Color
	subtle    color.Color
	accent    color.Color
	surface   color.Color
	onBrand   color.Color

	userColors []color.Color

	borderStyle lipgloss.Border

	base lipgloss.Style
}

// BasicTheme returns the default theme for a terminal with a dark (isDark)
// or light background. highlight optionally overrides the highlight colour.
func BasicTheme(isDark bool, highlight *string) Theme {
	lightDark := lipgloss.LightDark(isDark)
	base := Theme{isDark: isDark}

	base.border = lightDark(lipgloss.Color("#E2E8F0"), lipgloss.Color("#334155"))
	base.body = lightDark(lipgloss.Color("#64748B"), lipgloss.Color("#94A3B8"))
	base.subtle = lightDark(lipgloss.Color("#94A3B8"), lipgloss.Color("#64748B"))
	base.accent = lightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#F1F5F9"))
	base.surface = lightDark(lipgloss.Color("#F1F5F9"), lipgloss.Color("#1E1B2E"))
	base.brand = lightDark(lipgloss.Color("#7C3AED"), lipgloss.Color("#A78BFA"))
	base.secondary = lightDark(lipgloss.Color("#DB2777"), lipgloss.Color("#F472B6"))
	base.onBrand = lightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#13111C"))
	base.success = lightDark(lipgloss.Color("#059669"), lipgloss.Color("#34D399"))
	base.warning = lightDark(lipgloss.Color("#D97706"), lipgloss.Color("#FBBF24"))
	base.error = lightDark(lipgloss.Color("#DC2626"), lipgloss.Color("#F87171"))
	if highlight != nil {
		base.highlight = lipgloss.Color(*highlight)
	} else {
		base.highlight = base.brand
	}

	for _, pair := range [][2]string{
		{"#7C3AED", "#C4B5FD"}, // violet
		{"#DB2777", "#F9A8D4"}, // pink
		{"#0891B2", "#67E8F9"}, // cyan
		{"#059669", "#6EE7B7"}, // emerald
		{"#D97706", "#FCD34D"}, // amber
		{"#2563EB", "#93C5FD"}, // blue
		{"#E11D48", "#FDA4AF"}, // rose
		{"#65A30D", "#BEF264"}, // lime
	} {
		base.userColors = append(base.userColors, lightDark(lipgloss.Color(pair[0]), lipgloss.Color(pair[1])))
	}

	base.borderStyle = lipgloss.RoundedBorder()
	base.base = lipgloss.NewStyle().Foreground(base.body)

	return base
}

func (b Theme) IsDark() bool { return b.isDark }

func (b Theme) Body() color.Color      { return b.body }
func (b Theme) Subtle() color.Color    { return b.subtle }
func (b Theme) Highlight() color.Color { return b.highlight }
func (b Theme) Brand() color.Color     { return b.brand }
func (b Theme) Secondary() color.Color { return b.secondary }
func (b Theme) Accent() color.Color    { return b.accent }
func (b Theme) Surface() color.Color   { return b.surface }
func (b Theme) OnBrand() color.Color   { return b.onBrand }
func (b Theme) Success() color.Color   { return b.success }
func (b Theme) Warning() color.Color   { return b.warning }
func (b Theme) Error() color.Color     { return b.error }
func (b Theme) Border() color.Color    { return b.border }

// ToneColor is the foreground colour for a semantic tone.
func (b Theme) ToneColor(t Tone) color.Color {
	switch t {
	case ToneSuccess:
		return b.success
	case ToneWarning:
		return b.warning
	case ToneDanger:
		return b.error
	default:
		return b.brand
	}
}

// ToneIcon is the glyph shown next to a message of the given tone.
func ToneIcon(t Tone) string {
	switch t {
	case ToneSuccess:
		return "✓"
	case ToneWarning:
		return "!"
	case ToneDanger:
		return "✕"
	default:
		return "◆"
	}
}

// UserColor returns a stable colour for a username, so each participant keeps
// the same colour everywhere in the chat.
func (b Theme) UserColor(name string) color.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return b.userColors[h.Sum32()%uint32(len(b.userColors))]
}

func (b Theme) Base() lipgloss.Style {
	return b.base
}

func (b Theme) TextBody() lipgloss.Style {
	return b.Base().Foreground(b.body)
}

func (b Theme) TextSubtle() lipgloss.Style {
	return b.Base().Foreground(b.subtle)
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

func (b Theme) TextSecondary() lipgloss.Style {
	return b.Base().Foreground(b.secondary)
}

func (b Theme) TextError() lipgloss.Style {
	return b.Base().Foreground(b.error)
}

func (b Theme) TextSuccess() lipgloss.Style {
	return b.Base().Foreground(b.success)
}

func (b Theme) TextWarning() lipgloss.Style {
	return b.Base().Foreground(b.warning)
}

func (b Theme) PanelError() lipgloss.Style {
	return b.Base().Background(b.error).Foreground(b.onBrand)
}

// Key renders a keyboard shortcut, e.g. in the status bar.
func (b Theme) Key() lipgloss.Style {
	return b.Base().Foreground(b.brand).Bold(true)
}

// Pill renders a small badge with a coloured background.
func (b Theme) Pill(bg, fg color.Color) lipgloss.Style {
	return b.Base().Background(bg).Foreground(fg).Bold(true).Padding(0, 1)
}

// Card is a bordered panel.
func (b Theme) Card() lipgloss.Style {
	return b.Base().
		Border(b.borderStyle).
		BorderForeground(b.border).
		Padding(0, 1)
}

func (b Theme) Modal() lipgloss.Style {
	return b.Base().
		Border(b.borderStyle).
		BorderForeground(b.border).
		Foreground(b.accent).
		Align(lipgloss.Center)
}

// Gradient paints text with a left-to-right brand → secondary gradient.
// Multi-line text shares the same horizontal gradient on every line.
func (b Theme) Gradient(text string, bold bool) string {
	lines := strings.Split(text, "\n")
	width := 0
	for _, l := range lines {
		width = max(width, len([]rune(l)))
	}
	if width == 0 {
		return text
	}
	colors := lipgloss.Blend1D(max(width, 2), b.brand, b.secondary)
	out := make([]string, len(lines))
	for i, l := range lines {
		var sb strings.Builder
		for j, r := range []rune(l) {
			if r == ' ' {
				sb.WriteRune(' ')
				continue
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(colors[j]).Bold(bold).Render(string(r)))
		}
		out[i] = sb.String()
	}
	return strings.Join(out, "\n")
}
