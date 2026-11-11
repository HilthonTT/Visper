package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// keyHint is a "key description" pair shown in footers, status lines and
// dialogs.
type keyHint struct {
	key  string
	desc string
}

// keyHints renders "key desc · key desc · …", dropping trailing hints that
// don't fit in width (0 means unlimited).
func (m model) keyHints(hints []keyHint, width int) string {
	sep := m.theme.TextSubtle().Render(" · ")
	var out string
	for _, h := range hints {
		hint := m.theme.Key().Render(h.key)
		if h.desc != "" {
			hint += " " + m.theme.TextBody().Render(h.desc)
		}
		next := hint
		if out != "" {
			next = out + sep + hint
		}
		if width > 0 && lipgloss.Width(next) > width {
			break
		}
		out = next
	}
	return out
}

// button renders a dialog or page button; the focused one is filled in the
// tone's colour.
func (m model) button(label, hotkey string, focused bool, tone theme.Tone) string {
	text := label
	if hotkey != "" {
		text += " " + hotkey
	}
	if focused {
		return m.theme.Pill(m.theme.ToneColor(tone), m.theme.OnBrand()).Render(text)
	}
	return m.theme.Base().Background(m.theme.Surface()).Foreground(m.theme.Body()).Padding(0, 1).Render(text)
}

// newTextInput returns a single-line input in the app's style.
func (m model) newTextInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	styles := ti.Styles()
	for _, s := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
		s.Prompt = m.theme.TextBrand().Bold(true)
		s.Text = m.theme.TextAccent()
		s.Placeholder = m.theme.TextSubtle()
	}
	ti.SetStyles(styles)
	return ti
}

// rule is a horizontal line in the border colour.
func (m model) rule(width int) string {
	return m.theme.Base().Foreground(m.theme.Border()).Render(strings.Repeat("─", max(width, 0)))
}

// spread places left and right at the edges of a line of the given width.
func spread(width int, left, right string) string {
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// truncate shortens s to at most width cells, ending it with "…" when cut.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
