package tui

import (
	"encoding/json"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/embeds"
)

type faq struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type faqState struct {
	cursor int
}

// loadFaqs reads the embedded FAQ; it is part of the binary, so a failure is
// a build error and fatal.
func loadFaqs() []faq {
	data, err := embeds.JsonFaqData.ReadFile("faq.json")
	if err != nil {
		log.Fatalf("Failed to read embedded file: %s", err)
	}
	var faqs []faq
	if err := json.Unmarshal(data, &faqs); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %s", err)
	}
	return faqs
}

func (m model) gotoFaq() (model, tea.Cmd) {
	m = m.switchPage(faqPage)
	return m.setFooterHints(keyHint{"↑↓", "browse"}), nil
}

func (m model) faqUpdate(msg tea.Msg) (model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	n := len(m.faqs)
	if !ok || n == 0 {
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		m.state.faq.cursor = (m.state.faq.cursor - 1 + n) % n
	case "down":
		m.state.faq.cursor = (m.state.faq.cursor + 1) % n
	case "home", "g":
		m.state.faq.cursor = 0
	case "end", "G":
		m.state.faq.cursor = n - 1
	}
	return m, nil
}

// faqView is an accordion: the selected question is expanded, the others are
// one line each, and the list scrolls to keep the open answer on screen.
func (m model) faqView(height int) string {
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
