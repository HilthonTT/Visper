package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// Toasts are short, non-blocking notices that disappear on their own.

const toastDuration = 3 * time.Second

type toastState struct {
	id   int
	text string
	tone theme.Tone
}

// toastExpiredMsg hides toast id, unless a newer one replaced it.
type toastExpiredMsg struct{ id int }

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
