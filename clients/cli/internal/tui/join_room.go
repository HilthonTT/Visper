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

type joinRoomFailedMsg struct {
	err error
}

type joinRoomState struct {
	input   textinput.Model
	error   string
	joining bool
}

func (m model) JoinRoomSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(joinRoomPage)
	m = m.initJoinRoom()
	return m, textinput.Blink
}

func (m model) JoinRoomView(height int) string {
	s := m.state.joinRoom
	width := min(m.widthContainer-4, 52)

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

func (m model) JoinRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.state.joinRoom.joining {
			return m, nil
		}
		switch {
		case key.Matches(msg, keys.Back):
			// Clear the field first; a second esc goes back.
			if m.state.joinRoom.input.Value() != "" || m.state.joinRoom.error != "" {
				m.state.joinRoom.input.SetValue("")
				m.state.joinRoom.error = ""
				return m, nil
			}
			return m.NewRoomSwitch()
		case key.Matches(msg, keys.Enter):
			roomCode := strings.TrimSpace(m.state.joinRoom.input.Value())
			if roomCode == "" {
				m.state.joinRoom.error = "Enter a room code first."
				return m, nil
			}

			m.state.joinRoom.error = ""
			m.state.joinRoom.joining = true

			client, ctx, opts := m.client, m.context, m.requestOpts()
			return m, tea.Batch(animTick(animFastInterval), func() tea.Msg {
				room, err := client.Room.GetByJoinCode(ctx, apisdk.JoinByCodeParams{JoinCode: roomCode}, opts...)
				if err != nil {
					log.Printf("Failed to join room: %v", err)
					return joinRoomFailedMsg{err: err}
				}
				return roomJoinedMsg{room: room}
			})
		}
	case joinRoomFailedMsg:
		m.state.joinRoom.error = "No room matches that code. It may have expired, or the code changed."
		m.state.joinRoom.joining = false
		return m, nil
	}

	before := m.state.joinRoom.input.Value()
	m.state.joinRoom.input, cmd = m.state.joinRoom.input.Update(msg)
	if m.state.joinRoom.input.Value() != before {
		m.state.joinRoom.error = ""
	}
	return m, cmd
}

func (m model) initJoinRoom() model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "room code"
	ti.Focus()
	ti.CharLimit = 32
	ti.SetWidth(max(min(m.widthContainer-4, 52)-6, 10))
	styles := ti.Styles()
	for _, s := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
		s.Prompt = m.theme.TextBrand().Bold(true)
		s.Text = m.theme.TextAccent().Bold(true)
		s.Placeholder = m.theme.TextSubtle()
	}
	ti.SetStyles(styles)

	m.state.joinRoom = joinRoomState{input: ti}

	m.state.footer.commands = []footerCommand{
		{key: "enter", value: "join"},
		{key: "esc", value: "back"},
	}

	return m
}
