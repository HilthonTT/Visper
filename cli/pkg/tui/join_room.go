package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
)

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

func (m model) JoinRoomView() string {
	s := m.state.joinRoom

	var sections []string

	// Header
	header := m.theme.TextBrand().
		Bold(true).
		Render("Join Room")
	sections = append(sections, header)

	// Spacing
	sections = append(sections, "")

	// Instructions
	instructions := m.theme.TextBody().
		Render("Enter the room code to join an existing chat room.")
	sections = append(sections, instructions)

	// Spacing
	sections = append(sections, "")

	// Input label
	label := m.theme.TextAccent().
		Render("Room Code:")
	sections = append(sections, label)

	// Input field
	sections = append(sections, s.input.View())

	// Error message if any
	if s.error != "" {
		sections = append(sections, "")
		errorMsg := m.theme.TextError().
			Render("⚠ " + s.error)
		sections = append(sections, errorMsg)
	}

	// Joining status
	if s.joining {
		sections = append(sections, "")
		joiningMsg := m.theme.TextHighlight().
			Render("Joining room...")
		sections = append(sections, joiningMsg)
	}

	// Spacing before help
	sections = append(sections, "")
	sections = append(sections, "")

	// Help text
	help := m.theme.TextBody().
		Faint(true).
		Render("Press Enter to join • Esc to go back")
	sections = append(sections, help)

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Center the content
	containerStyle := m.theme.Base().
		Width(m.widthContent).
		Height(m.heightContent).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center)

	return containerStyle.Render(content)
}

func (m model) JoinRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Enter):
			roomCode := strings.TrimSpace(m.state.joinRoom.input.Value())
			if roomCode == "" {
				m.state.joinRoom.error = "Room code cannot be empty"
				return m, nil
			}

			m.state.joinRoom.error = ""
			m.state.joinRoom.joining = true

			opts := []option.RequestOption{}
			if m.userID != nil && *m.userID != "" {
				opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
			}

			roomToJoin, err := m.client.Room.GetByJoinCode(m.context, apisdk.JoinByCodeParams{
				JoinCode: roomCode,
				Username: "",
			}, opts...)

			if err != nil {
				m.state.joinRoom.error = "Failed to join room"
				m.state.joinRoom.joining = false
				return m, nil
			}

			return m.ChatSwitch(roomToJoin)
		}
	}

	m.state.joinRoom.input, cmd = m.state.joinRoom.input.Update(msg)
	return m, cmd
}

func (m model) initJoinRoom() model {
	ti := textinput.New()
	ti.Placeholder = "Enter room code..."
	ti.Focus()
	ti.CharLimit = 32
	ti.Width = 40
	ti.PromptStyle = m.theme.TextBrand()
	ti.TextStyle = m.theme.TextAccent()
	ti.PlaceholderStyle = m.theme.TextBody()

	m.state.joinRoom = joinRoomState{
		input:   ti,
		error:   "",
		joining: false,
	}

	m.state.footer.commands = []footerCommand{
		{key: "enter"},
		{key: "esc"},
	}

	return m
}
