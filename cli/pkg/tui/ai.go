package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
)

type aiEnhanceResultMsg struct {
	enhanced string
	err      error
}

func (m model) enhanceMessage(content string) tea.Cmd {
	return func() tea.Msg {
		opts := []option.RequestOption{}
		if m.userID != nil && *m.userID != "" {
			opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
		}

		res, err := m.client.AI.Enhance(m.context, apisdk.AIEnhanceRequest{
			Message: content,
			Style:   m.state.chat.aiEnhanceStyle,
			Tone:    m.state.chat.aiEnhanceTone,
		}, opts...)

		if err != nil {
			return aiEnhanceResultMsg{err: err}
		}
		return aiEnhanceResultMsg{enhanced: res.Enhanced}
	}
}
