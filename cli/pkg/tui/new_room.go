package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type newRoomState struct {
	creating bool
	roomCode string
	error    string
}

func (m model) NewRoomSwitch() (model, tea.Cmd) {
	m.state.newRoom.creating = false
	m.state.newRoom.roomCode = ""

	m = m.SwitchPage(newRoomPage)
	m = m.initNewRoom()

	return m, nil
}

func (m model) NewRoomView() string {
	s := m.state.newRoom

	var sections []string

	// Header
	header := m.theme.TextBrand().
		Bold(true).
		Render("Create New Room")
	sections = append(sections, header)

	// Spacing
	sections = append(sections, "")

	// Description
	if !s.creating && s.roomCode == "" {
		description := m.theme.TextBody().
			Render("Create a new anonymous chat room and get a unique room code\nthat you can share with others.")
		sections = append(sections, description)

		// Spacing
		sections = append(sections, "")
		sections = append(sections, "")

		// Confirmation prompt
		prompt := m.theme.TextAccent().
			Bold(true).
			Render("Do you want to create a new room?")
		sections = append(sections, prompt)

		// Spacing
		sections = append(sections, "")

		// Options
		yesOption := m.theme.TextHighlight().
			Render("  [Y] Yes, create room")
		noOption := m.theme.TextBody().
			Render("  [N] No, go back")

		sections = append(sections, yesOption)
		sections = append(sections, noOption)
	}

	// Creating status
	if s.creating {
		sections = append(sections, "")
		creatingMsg := m.theme.TextHighlight().
			Render("Creating room...")

		spinner := m.theme.TextBrand().
			Render("⣾⣽⣻⢿⡿⣟⣯⣷") // You can animate this

		sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Left, spinner, "  ", creatingMsg))
	}

	// Success - Room created
	if s.roomCode != "" {
		sections = append(sections, "")

		successMsg := m.theme.TextHighlight().
			Bold(true).
			Render("✓ Room created successfully!")
		sections = append(sections, successMsg)

		sections = append(sections, "")

		// Room code display
		codeLabel := m.theme.TextBody().
			Render("Your room code:")
		sections = append(sections, codeLabel)

		sections = append(sections, "")

		// Room code in a box
		codeStyle := m.theme.Base().
			Foreground(m.theme.Accent()).
			Background(m.theme.Highlight()).
			Bold(true).
			Padding(1, 3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Highlight())

		codeBox := codeStyle.Render(s.roomCode)
		sections = append(sections, codeBox)

		sections = append(sections, "")

		instruction := m.theme.TextBody().
			Faint(true).
			Render("Share this code with others to let them join your room.")
		sections = append(sections, instruction)

		sections = append(sections, "")

		// Action prompt
		nextAction := m.theme.TextAccent().
			Render("Press Enter to continue...")
		sections = append(sections, nextAction)
	}

	if s.error != "" {
		sections = append(sections, "")
		errorMsg := m.theme.TextError().
			Render("⚠ " + s.error)
		sections = append(sections, errorMsg)
	}

	// Spacing before help
	if !s.creating && s.roomCode == "" {
		sections = append(sections, "")
		sections = append(sections, "")

		// Help text
		help := m.theme.TextBody().
			Faint(true).
			Render("Y to confirm • N or Esc to cancel")
		sections = append(sections, help)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Center the content
	containerStyle := m.theme.Base().
		Width(m.widthContent).
		Height(m.heightContent).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center)

	return containerStyle.Render(content)
}

func (m model) NewRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	s := m.state.newRoom

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.roomCode != "" {
			switch {
			case key.Matches(msg, keys.Enter):
				// TODO: Navigate to the chat page
			}
			return m, nil
		}

		if s.creating {
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Yes):
			m.state.newRoom.creating = true
			m.state.newRoom.error = ""

			// TODO: Implement actual room creation logic
			// return m, m.createRoomCmd()

			// For now, simulate success (remove this in production)
			// m.state.newRoom.roomCode = "ABC-123-XYZ"
			// m.state.newRoom.creating = false

			return m.ChatSwitch()
		}
	}

	return m, nil
}

func (m model) initNewRoom() model {
	m.state.newRoom = newRoomState{
		creating: false,
		roomCode: "",
		error:    "",
	}

	m.state.footer.commands = []footerCommand{
		{key: "y"},
		{key: "n/esc"},
	}

	return m
}
