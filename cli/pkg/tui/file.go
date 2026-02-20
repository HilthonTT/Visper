package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type fileUploadResultMsg struct {
	fileURL  string
	filename string
	err      error
}

func (m model) uploadFile(filePath string) tea.Cmd {
	return func() tea.Msg {
		userID := ""
		if m.userID != nil {
			userID = *m.userID
		}

		if m.state.chat.room == nil {
			return fileUploadResultMsg{err: fmt.Errorf("not in a room")}
		}

		result, err := m.client.File.Upload(
			m.context,
			m.state.chat.room.ID,
			filePath,
			userID,
		)
		if err != nil {
			return fileUploadResultMsg{err: err}
		}

		return fileUploadResultMsg{
			fileURL:  result.URL,
			filename: result.Filename,
		}
	}
}
