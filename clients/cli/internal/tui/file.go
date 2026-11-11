package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type fileUploadResultMsg struct {
	fileURL  string
	filename string
	err      error
	// session is the chat session (its wsCtx) the upload was started in; the
	// result auto-sends a message, which must not land in a room joined later.
	session context.Context
}

func (m model) uploadFile(filePath string) tea.Cmd {
	session := m.state.chat.wsCtx
	if m.state.chat.room == nil {
		return func() tea.Msg {
			return fileUploadResultMsg{err: fmt.Errorf("not in a room"), session: session}
		}
	}
	client, ctx, roomID := m.client, m.context, m.state.chat.room.ID
	var userID string
	if m.userID != nil {
		userID = *m.userID
	}
	return func() tea.Msg {
		result, err := client.File.Upload(ctx, roomID, filePath, userID)
		if err != nil {
			return fileUploadResultMsg{err: err, session: session}
		}
		return fileUploadResultMsg{
			fileURL:  result.URL,
			filename: result.Filename,
			session:  session,
		}
	}
}
