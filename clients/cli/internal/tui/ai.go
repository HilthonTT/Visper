package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

type aiEnhanceResultMsg struct {
	enhanced string
	err      error
	// session is the chat session (its wsCtx) the draft belongs to, so a
	// rewrite that finishes after leaving the room isn't put in another
	// room's message box.
	session context.Context
}

func (m model) enhanceMessage(content string) tea.Cmd {
	client, ctx, opts, session := m.client, m.context, m.requestOpts(), m.state.chat.wsCtx
	req := apisdk.AIEnhanceRequest{
		Message: content,
		Style:   m.state.chat.aiEnhanceStyle,
		Tone:    m.state.chat.aiEnhanceTone,
	}
	return func() tea.Msg {
		res, err := client.AI.Enhance(ctx, req, opts...)
		if err != nil {
			return aiEnhanceResultMsg{err: err, session: session}
		}
		return aiEnhanceResultMsg{enhanced: res.Enhanced, session: session}
	}
}
