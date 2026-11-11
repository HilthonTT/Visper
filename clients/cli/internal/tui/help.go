package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The help screen is an overlay listing every shortcut; any key closes it.

type helpSection struct {
	title string
	keys  []keyHint
}

var helpSections = []helpSection{
	{"Anywhere", []keyHint{
		{"tab / ←→", "switch page"},
		{"n j f s", "new · join · faq · settings"},
		{"m", "menu"},
		{"?", "this help"},
		{"ctrl+c", "quit"},
	}},
	{"In a room", []keyHint{
		{"enter", "send message"},
		{"pgup/pgdn", "scroll history"},
		{"ctrl+e", "edit / delete your messages"},
		{"ctrl+u", "share an image"},
		{"ctrl+a", "rewrite draft with AI"},
		{"ctrl+s", "search members"},
		{"ctrl+p", "QR invite"},
		{"ctrl+n", "new join code (owner)"},
		{"ctrl+w", "show / hide companion"},
		{"ctrl+b", "leave room"},
	}},
}

func (m model) helpView() string {
	width := min(56, m.viewportWidth-4)
	inner := width - 4

	keyWidth := 0
	for _, s := range helpSections {
		for _, k := range s.keys {
			keyWidth = max(keyWidth, lipgloss.Width(k.key))
		}
	}

	var parts []string
	for i, s := range helpSections {
		if i > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, m.theme.TextSecondary().Bold(true).Render(s.title))
		for _, k := range s.keys {
			parts = append(parts,
				m.theme.Key().Width(keyWidth+2).Render(k.key)+m.theme.TextBody().Render(k.desc))
		}
	}
	parts = append(parts, "", m.theme.TextSubtle().Render("press any key to close"))

	// Drop lines that don't fit on short terminals.
	if maxLines := max(m.viewportHeight-6, 3); len(parts) > maxLines {
		parts = parts[:maxLines]
	}

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 1).
		Width(width).
		Render(m.theme.Base().Width(inner).Render(strings.Join(parts, "\n")))
}
