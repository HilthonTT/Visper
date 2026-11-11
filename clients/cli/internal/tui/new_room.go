package tui

import (
	"log"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

type createRoomFailedMsg struct {
	err error
}

type newRoomState struct {
	creating bool
	error    string
}

func (m model) NewRoomSwitch() (model, tea.Cmd) {
	m = m.SwitchPage(newRoomPage)
	m = m.initNewRoom()

	// Reconnect the notification WebSocket if we came from chat (where it was
	// disconnected).
	return m.ensureNotificationWebSocket()
}

// pageContent centres a page's content in the body area.
func (m model) pageContent(height int, content string) string {
	return lipgloss.Place(m.widthContainer, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) NewRoomView(height int) string {
	s := m.state.newRoom
	width := min(m.widthContainer-4, 60)

	title := m.theme.TextAccent().Bold(true).Render("Start a new room")
	lead := m.theme.TextBody().Width(width).Render("A private room for you and whoever you invite. Share the code, chat, and it's gone.")

	check := m.theme.TextSuccess().Render("✓ ")
	features := lipgloss.JoinVertical(lipgloss.Left,
		check+m.theme.TextBody().Render("Encrypted on your device; the server sees ciphertext"),
		check+m.theme.TextBody().Render("No account, email or phone number"),
		check+m.theme.TextBody().Render("Disappears after 24 hours"),
	)

	var action string
	switch {
	case s.creating:
		action = m.spinner() + " " + m.theme.TextBrand().Render("Creating your room…")
	default:
		action = m.button("Create room", "↵", true, theme.ToneInfo)
	}

	parts := []string{title, "", lead, "", features, "", action}
	if s.error != "" {
		parts = append(parts, "", m.theme.TextError().Width(width).Render("✕ "+s.error))
	}

	return m.pageContent(height, m.theme.Base().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, parts...)))
}

func (m model) NewRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.state.newRoom.creating {
			return m, nil
		}

		switch msg.String() {
		case "enter", "y", "Y":
			m.state.newRoom.creating = true
			m.state.newRoom.error = ""

			client, ctx, opts := m.client, m.context, m.requestOpts()
			return m, tea.Batch(animTick(animFastInterval), func() tea.Msg {
				// Create room with 24 hour expiry
				newRoom, err := client.Room.Create(ctx, apisdk.RoomCreateParams{ExpiryHours: 24}, opts...)
				if err != nil {
					log.Printf("Failed to create room: %v", err)
					return createRoomFailedMsg{err: err}
				}
				return roomJoinedMsg{room: newRoom}
			})
		}
	case createRoomFailedMsg:
		m.state.newRoom.creating = false
		m.state.newRoom.error = "Couldn't create the room. Is the server running? Press enter to try again."
		return m, nil
	}

	return m, nil
}

func (m model) initNewRoom() model {
	m.state.newRoom = newRoomState{}
	m.state.footer.commands = []footerCommand{
		{key: "enter", value: "create room"},
	}
	return m
}
