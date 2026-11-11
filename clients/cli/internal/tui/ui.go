package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// ---------------------------------------------------------------------------
// Key hints

// keyHints renders "key desc · key desc · …", dropping trailing hints that
// don't fit in width.
func (m model) keyHints(cmds []footerCommand, width int) string {
	sep := m.theme.TextSubtle().Render(" · ")
	var out string
	for _, c := range cmds {
		hint := m.theme.Key().Render(c.key)
		if c.value != "" {
			hint += " " + m.theme.TextBody().Render(c.value)
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

// ---------------------------------------------------------------------------
// Overlays

// overlay draws fg centred on top of bg, with a one-cell clear margin so the
// page behind doesn't butt up against its border.
func (m model) overlay(fg, bg string) string {
	fg = lipgloss.NewStyle().Padding(0, 1).Render(fg)
	x := max((m.viewportWidth-lipgloss.Width(fg))/2, 0)
	y := max((m.viewportHeight-lipgloss.Height(fg))/2, 0)
	return strfn.PlaceOverlay(x, y, fg, bg)
}

// ---------------------------------------------------------------------------
// Toasts: short, non-blocking notices that disappear on their own.

type toastState struct {
	id   int
	text string
	tone theme.Tone
}

type toastExpiredMsg struct{ id int }

const toastDuration = 3 * time.Second

func (m model) toast(tone theme.Tone, text string) (model, tea.Cmd) {
	id := m.state.toast.id + 1
	m.state.toast = toastState{id: id, text: text, tone: tone}
	return m, tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastExpiredMsg{id: id} })
}

func (m model) toastView(maxWidth int) string {
	t := m.state.toast
	if t.text == "" {
		return ""
	}
	c := m.theme.ToneColor(t.tone)
	icon := m.theme.Base().Foreground(c).Bold(true).Render(theme.ToneIcon(t.tone))
	text := m.theme.TextAccent().Render(ansi.Truncate(t.text, max(maxWidth-8, 4), "…"))
	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c).
		Padding(0, 1).
		Render(icon + " " + text)
}

// withToastAt places the toast, if any, with its right edge at column right
// and its bottom edge bottom rows above the bottom of view.
func (m model) withToastAt(view string, right, bottom int) string {
	t := m.toastView(min(m.viewportWidth-4, 64))
	if t == "" {
		return view
	}
	x := max(right-lipgloss.Width(t), 0)
	y := max(m.viewportHeight-lipgloss.Height(t)-bottom, 0)
	return strfn.PlaceOverlay(x, y, t, view)
}

// ---------------------------------------------------------------------------
// Misc

// rule is a horizontal line in the border colour.
func (m model) rule(width int) string {
	return m.theme.Base().Foreground(m.theme.Border()).Render(strings.Repeat("─", max(width, 0)))
}

// spread places left and right at the edges of a line of the given width.
func spread(width int, left, right string) string {
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}
