package tui

import (
	"encoding/json"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hilthontt/visper/cli/pkg/tui/embeds"
)

type FAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
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
	return m, nil
}

func (m model) FaqView() string {
	var faqs []string
	for _, faq := range m.faqs {
		faqs = append(
			faqs,
			m.theme.TextAccent().Render(wordWrap(faq.Question, m.widthContent)),
		)
		faqs = append(
			faqs,
			m.theme.Base().Render(wordWrap(faq.Answer, m.widthContent)),
		)
		faqs = append(faqs, "")
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		faqs...,
	)
}
