package tui

import (
	"encoding/json"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/embeds"
)

type FAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type faqState struct {
	cursor int
}

func LoadFaqs() []FAQ {
	data, err := embeds.JsonFaqData.ReadFile("faq.json")
	if err != nil {
		log.Fatalf("Failed to read embedded file: %s", err)
	}
	var faqs []FAQ
	if err := json.Unmarshal(data, &faqs); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %s", err)
	}
	return faqs
}

func (m model) FaqSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(faqPage)
	m.state.footer.commands = []footerCommand{{key: "↑↓", value: "browse"}}
	return m, nil
}

func (m model) FaqUpdate(msg tea.Msg) (model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || len(m.faqs) == 0 {
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		m.state.faq.cursor = (m.state.faq.cursor - 1 + len(m.faqs)) % len(m.faqs)
	case "down":
		m.state.faq.cursor = (m.state.faq.cursor + 1) % len(m.faqs)
	case "home", "g":
		m.state.faq.cursor = 0
	case "end", "G":
		m.state.faq.cursor = len(m.faqs) - 1
	}
	return m, nil
}

// FaqView is an accordion: the selected question is expanded, the others are
// one line each, and the list scrolls to keep the open answer on screen.
func (m model) FaqView(height int) string {
	width := min(m.widthContainer-2, 76)

	var lines []string
	selStart, selEnd := 0, 0
	for i, f := range m.faqs {
		if i == m.state.faq.cursor {
			selStart = len(lines)
			q := m.theme.TextBrand().Bold(true).Render("▾ ") +
				m.theme.TextAccent().Bold(true).Width(width-2).Render(f.Question)
			a := m.theme.TextBody().PaddingLeft(2).Width(width).Render(f.Answer)
			block := lipgloss.JoinVertical(lipgloss.Left, q, a, "")
			lines = append(lines, strings.Split(block, "\n")...)
			selEnd = len(lines) - 1
			continue
		}
		q := m.theme.TextSubtle().Render("▸ ") + m.theme.TextBody().Render(truncate(f.Question, width-2))
		lines = append(lines, q)
	}

	// Leave a line above for breathing room and keep the selection visible.
	height = max(height-1, 1)
	start := 0
	if selEnd >= height {
		start = selEnd - height + 1
	}
	start = min(start, selStart)
	end := min(start+height, len(lines))

	body := strings.Join(lines[start:end], "\n")
	return lipgloss.Place(m.widthContainer, height+1, lipgloss.Center, lipgloss.Top,
		"\n"+m.theme.Base().Width(width).Render(body))
}

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
