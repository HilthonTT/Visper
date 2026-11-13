package tui

import (
	"log"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// roomExpiryHours is how long a newly created room lives.
const roomExpiryHours = 24

type newRoomState struct {
	creating bool
	error    string
}

type createRoomFailedMsg struct {
	err error
}

func (m model) gotoNewRoom() (model, tea.Cmd) {
	m = m.switchPage(newRoomPage)
	m.state.newRoom = newRoomState{}
	m = m.setFooterHints(keyHint{"enter", "create room"})

	// Reconnect the notification listener if we came from a room (where it
	// is stopped).
	return m.startNotificationListener()
}

func (m model) newRoomUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.state.newRoom.creating {
			return m, nil
		}
		if key.Matches(msg, keys.Enter) || msg.String() == "y" || msg.String() == "Y" {
			return m.createRoom()
		}
	case createRoomFailedMsg:
		m.state.newRoom.creating = false
		m.state.newRoom.error = "Couldn't create the room. Is the server running? Press enter to try again."
	}
	return m, nil
}

func (m model) createRoom() (model, tea.Cmd) {
	m.state.newRoom.creating = true
	m.state.newRoom.error = ""

	client, ctx, opts := m.client, m.ctx, m.requestOpts()
	anim := m.restartAnim()
	return m, tea.Batch(anim, func() tea.Msg {
		// The key is made here and never sent to the server.
		key, err := apisdk.NewRoomKey()
		if err != nil {
			log.Printf("Failed to create room key: %v", err)
			return createRoomFailedMsg{err: err}
		}
		room, err := client.Room.Create(ctx, apisdk.RoomCreateParams{ExpiryHours: roomExpiryHours}, opts...)
		if err != nil {
			log.Printf("Failed to create room: %v", err)
			return createRoomFailedMsg{err: err}
		}
		return roomJoinedMsg{room: room, roomKey: key}
	})
}

func (m model) newRoomView(height int) string {
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

	action := m.button("Create room", "↵", true, theme.ToneInfo)
	if s.creating {
		action = m.spinner() + " " + m.theme.TextBrand().Render("Creating your room…")
	}

	parts := []string{title, "", lead, "", features, "", action}
	if s.error != "" {
		parts = append(parts, "", m.theme.TextError().Width(width).Render("✕ "+s.error))
	}

	return m.pageContent(height, m.theme.Base().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, parts...)))
}
