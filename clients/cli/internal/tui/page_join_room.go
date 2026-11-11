package tui

import (
	"log"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

const joinRoomMaxWidth = 52

type joinRoomState struct {
	input   textinput.Model
	error   string
	joining bool
}

type joinRoomFailedMsg struct {
	err error
}

func (m model) gotoJoinRoom() (model, tea.Cmd) {
	m = m.switchPage(joinRoomPage)

	input := m.newTextInput("room code")
	input.CharLimit = 32
	input.SetWidth(max(min(m.widthContainer-4, joinRoomMaxWidth)-6, 10))
	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.Bold(true)
	styles.Blurred.Text = styles.Blurred.Text.Bold(true)
	input.SetStyles(styles)
	input.Focus()

	m.state.joinRoom = joinRoomState{input: input}
	m = m.setFooterHints(keyHint{"enter", "join"}, keyHint{"esc", "back"})
	return m, textinput.Blink
}

func (m model) joinRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	s := &m.state.joinRoom

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if s.joining {
			return m, nil
		}
		switch {
		case key.Matches(msg, keys.Back):
			// Clear the field first; a second esc goes back.
			if s.input.Value() != "" || s.error != "" {
				s.input.SetValue("")
				s.error = ""
				return m, nil
			}
			return m.gotoNewRoom()
		case key.Matches(msg, keys.Enter):
			return m.joinRoom()
		}
	case joinRoomFailedMsg:
		s.error = "No room matches that code. It may have expired, or the code changed."
		s.joining = false
		return m, nil
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		s.error = ""
	}
	return m, cmd
}

func (m model) joinRoom() (model, tea.Cmd) {
	code := strings.TrimSpace(m.state.joinRoom.input.Value())
	if code == "" {
		m.state.joinRoom.error = "Enter a room code first."
		return m, nil
	}
	m.state.joinRoom.error = ""
	m.state.joinRoom.joining = true

	client, ctx, opts := m.client, m.ctx, m.requestOpts()
	return m, tea.Batch(animTick(animFastInterval), func() tea.Msg {
		room, err := client.Room.GetByJoinCode(ctx, apisdk.JoinByCodeParams{JoinCode: code}, opts...)
		if err != nil {
			log.Printf("Failed to join room: %v", err)
			return joinRoomFailedMsg{err: err}
		}
		return roomJoinedMsg{room: room}
	})
}

func (m model) joinRoomView(height int) string {
	s := m.state.joinRoom
	width := min(m.widthContainer-4, joinRoomMaxWidth)

	title := m.theme.TextAccent().Bold(true).Render("Join a room")
	lead := m.theme.TextBody().Width(width).Render("Type or paste the code someone shared with you.")

	borderColor := m.theme.Brand()
	if s.error != "" {
		borderColor = m.theme.Error()
	}
	field := m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width).
		Render(s.input.View())

	var status string
	switch {
	case s.joining:
		status = m.spinner() + " " + m.theme.TextBrand().Render("Joining…")
	case s.error != "":
		status = m.theme.TextError().Width(width).Render("✕ " + s.error)
	default:
		status = m.theme.TextSubtle().Render("press enter to join")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", lead, "", field, status)
	return m.pageContent(height, m.theme.Base().Width(width).Render(content))
}
