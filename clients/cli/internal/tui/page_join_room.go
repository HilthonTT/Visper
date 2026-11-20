package tui

import (
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

const joinRoomMaxWidth = 52

type joinRoomState struct {
	input   textinput.Model
	error   string
	joining bool
	// rooms are the remembered rooms to reopen (see identity.go); cursor
	// is the highlighted one, or -1 for the invite code field.
	rooms  []settings.StoredRoom
	cursor int
}

// maxListedRooms is how many remembered rooms the join page shows.
const maxListedRooms = 5

type joinRoomFailedMsg struct {
	err error
}

func (m model) gotoJoinRoom() (model, tea.Cmd) {
	m = m.switchPage(joinRoomPage)

	input := m.newTextInput("invite code")
	input.CharLimit = 96
	input.SetWidth(max(min(m.widthContainer-4, joinRoomMaxWidth)-6, 10))
	styles := input.Styles()
	styles.Focused.Text = styles.Focused.Text.Bold(true)
	styles.Blurred.Text = styles.Blurred.Text.Bold(true)
	input.SetStyles(styles)
	input.Focus()

	rooms := m.rememberedRooms()
	if len(rooms) > maxListedRooms {
		rooms = rooms[:maxListedRooms]
	}
	m.state.joinRoom = joinRoomState{input: input, rooms: rooms, cursor: -1}
	if len(rooms) > 0 {
		m = m.setFooterHints(keyHint{"enter", "join"}, keyHint{"↑↓", "your rooms"}, keyHint{"esc", "back"})
	} else {
		m = m.setFooterHints(keyHint{"enter", "join"}, keyHint{"esc", "back"})
	}
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
		case msg.String() == "up" && len(s.rooms) > 0:
			s.cursor = max(s.cursor-1, -1)
			return m, nil
		case msg.String() == "down" && len(s.rooms) > 0:
			s.cursor = min(s.cursor+1, len(s.rooms)-1)
			return m, nil
		case key.Matches(msg, keys.Enter) && s.cursor >= 0:
			s.error = ""
			s.joining = true
			reopen := m.reopenRoom(s.rooms[s.cursor])
			anim := m.restartAnim()
			return m, tea.Batch(anim, reopen)
		case key.Matches(msg, keys.Back) && s.cursor >= 0:
			s.cursor = -1
			return m, nil
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
		s.error = "No room matches that invite code. It may have expired, or the code changed."
		s.joining = false
		return m, nil
	case roomReopenFailedMsg:
		s.joining = false
		if !msg.gone {
			log.Printf("Failed to reopen room %s: %v", msg.roomID, msg.err)
			s.error = "Couldn't reach the server to reopen that room. Try again in a moment."
			return m, nil
		}
		m.forgetRoom(msg.roomID)
		s.rooms = slices.DeleteFunc(s.rooms, func(r settings.StoredRoom) bool { return r.ID == msg.roomID })
		s.cursor = min(s.cursor, len(s.rooms)-1)
		s.error = "That room is gone: it expired, was deleted, or you were removed. It was forgotten."
		return m, nil
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		s.error = ""
		s.cursor = -1 // typing goes back to the invite code
	}
	return m, cmd
}

func (m model) joinRoom() (model, tea.Cmd) {
	input := strings.TrimSpace(m.state.joinRoom.input.Value())
	if input == "" {
		m.state.joinRoom.error = "Paste an invite code first."
		return m, nil
	}
	code, key, err := apisdk.ParseInviteCode(input)
	switch {
	case errors.Is(err, apisdk.ErrMissingRoomKey):
		m.state.joinRoom.error = "That's only the join code. Ask for the full invite code (ctrl+p in their room): it carries the room's key."
		return m, nil
	case err != nil:
		m.state.joinRoom.error = "That invite code is incomplete or mistyped. Copy it again."
		return m, nil
	}
	m.state.joinRoom.error = ""
	m.state.joinRoom.joining = true

	client, ctx, opts := m.client, m.ctx, m.requestOpts()
	anim := m.restartAnim()
	return m, tea.Batch(anim, func() tea.Msg {
		room, err := client.Room.GetByJoinCode(ctx, apisdk.JoinByCodeParams{JoinCode: code}, opts...)
		if err != nil {
			log.Printf("Failed to join room: %v", err)
			return joinRoomFailedMsg{err: err}
		}
		return roomJoinedMsg{room: room, roomKey: key}
	})
}

func (m model) joinRoomView(height int) string {
	s := m.state.joinRoom
	width := min(m.widthContainer-4, joinRoomMaxWidth)

	title := m.theme.TextAccent().Bold(true).Render("Join a room")
	lead := m.theme.TextBody().Width(width).Render("Paste the invite code someone shared with you. It carries the room's key, which the server never sees.")

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

	parts := []string{title, "", lead, "", field, status}
	if len(s.rooms) > 0 {
		parts = append(parts, "", m.theme.TextSecondary().Bold(true).Render("Your rooms"),
			m.theme.TextSubtle().Width(width).Render("Rooms you're still in. Reopen one without an invite: ↑↓ then enter."))
		for i, r := range s.rooms {
			parts = append(parts, m.rememberedRoomRow(r, i == s.cursor, width))
		}
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.pageContent(height, m.theme.Base().Width(width).Render(content))
}

// rememberedRoomRow is one remembered room: its join code, who owns it and
// when it expires.
func (m model) rememberedRoomRow(r settings.StoredRoom, selected bool, width int) string {
	owner := r.OwnerName
	switch {
	case m.isCurrentUser(r.OwnerID):
		owner = "you"
	case owner == "":
		owner = "someone"
	}
	left := "#" + r.JoinCode
	if r.JoinCode == "" {
		left = "room " + truncate(r.ID, 8)
	}
	meta := "owner " + owner
	if !r.ExpiresAt.IsZero() {
		meta += " · " + formatDuration(time.Until(r.ExpiresAt)) + " left"
	}
	row := spread(width-2, m.theme.TextAccent().Bold(true).Render(left), m.theme.TextSubtle().Render(truncate(meta, max(width-14, 8))))
	if selected {
		return m.theme.TextSecondary().Bold(true).Render("▌ ") + m.theme.Base().Background(m.theme.Surface()).Render(row)
	}
	return "  " + row
}
