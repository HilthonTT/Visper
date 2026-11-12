package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// roomDeletedDelay and kickedDelay are how long the dialog saying so stays up
// before returning to the start page.
const (
	roomDeletedDelay = 2 * time.Second
	kickedDelay      = 3 * time.Second
)

var sentMoods = []string{
	"nice one~",
	"they'll love that ♡",
	"keep it up!",
	"ooh, spicy",
	"so eloquent ✧",
}

// chatEventUpdate handles room events and the results of background work;
// handled is false for any other message.
func (m model) chatEventUpdate(msg tea.Msg) (_ model, _ tea.Cmd, handled bool) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	// Connection
	case roomConnectedMsg:
		if msg.session != m.state.chat.session || msg.session.Err() != nil {
			// We left the room while the connection was being established.
			msg.conn.Close()
			return m, nil, true
		}
		m.state.chat.ws = msg.conn
		return m, listenRoom(msg.session, msg.conn, m.state.chat.roomKey), true
	case roomConnectFailedMsg:
		if msg.session != m.state.chat.session || msg.session.Err() != nil {
			return m, nil, true // we already left this room
		}
		// Messages only arrive over the WebSocket, so without it the room
		// would just look silent.
		return m.alert(theme.ToneDanger, "Couldn't connect",
			"Live updates for this room are unavailable, so new messages won't show up. Press ctrl+b to leave and try again."), nil, true
	case roomListeningMsg:
		if msg.session != m.state.chat.session {
			return m, nil, true // listener of a room we already left
		}
		m.state.chat.events = msg.events
		return m, waitForRoomEvent(msg.events), true
	case roomDisconnectedMsg:
		return m.alert(theme.ToneDanger, "Disconnected", "Lost the connection to this room. Press ctrl+b to leave."), nil, true
	case roomErrorMsg:
		m, cmd = m.roomErrorUpdate(msg)

	// Messages
	case messageReceivedMsg:
		m, cmd = m.messageReceivedUpdate(msg)
	case messageUpdatedMsg:
		for i := range m.state.chat.messages {
			if m.state.chat.messages[i].ID == msg.messageID {
				m.state.chat.messages[i].Content = msg.content
				break
			}
		}
		m = m.refreshMessages()
		cmd = m.waitForRoomEvent()
	case messageDeletedMsg:
		m = m.removeMessage(msg.messageID).refreshMessages()
		cmd = m.waitForRoomEvent()

	// Members
	case memberJoinedMsg:
		m, cmd = m.memberJoinedUpdate(msg)
	case memberLeftMsg:
		name := msg.username
		if name == "" {
			name = m.state.chat.members.name(msg.userID)
		}
		m.state.chat.members.remove(msg.userID)
		if name != "" {
			m.state.chat.companion.mood = "bye bye~"
			m, cmd = m.toast(theme.ToneInfo, name+" left")
		}
		cmd = tea.Batch(cmd, m.waitForRoomEvent())
	case memberListMsg:
		s := &m.state.chat.members
		s.list = msg.members
		s.resetFilter()
		// Keep an active member search applied to the new list.
		if q := s.search.Value(); s.searching && q != "" {
			cmd = m.searchMembers(q)
		}
		cmd = tea.Batch(cmd, m.waitForRoomEvent())
	case memberSearchResultMsg:
		s := &m.state.chat.members
		if !s.searching || msg.query != s.search.Value() {
			// Superseded by later typing, or the search was closed meanwhile;
			// applying it would leave the list filtered by an old query.
			return m, nil, true
		}
		if msg.query == "" {
			s.resetFilter()
		} else {
			s.applySearchResult(msg.ids)
		}
		s.cursor = 0
	case kickedMsg:
		m, cmd = m.kickedUpdate(msg)

	// Room
	case roomUpdatedMsg:
		changed := m.state.chat.roomCode != "" && m.state.chat.roomCode != msg.joinCode
		m.state.chat.roomCode = msg.joinCode
		m = m.withJoinCodeInQRURL(msg.joinCode)
		if changed {
			m, cmd = m.toast(theme.ToneSuccess, "New join code: "+msg.joinCode+". Press ctrl+p for the new invite.")
		}
		cmd = tea.Batch(cmd, m.waitForRoomEvent())
	case roomDeletedMsg:
		m = m.alert(theme.ToneDanger, "Room deleted", "The owner deleted this room. Taking you back to the start…")
		cmd = m.closeRoomAfter(roomDeletedDelay)
	case expiryTickMsg:
		m, cmd = m.expiryTickUpdate(msg)
	case roomExpiredMsg:
		m, cmd = m.roomExpiredUpdate(msg)
	case roomClosedMsg:
		m, cmd = m.closeRoom(msg.session)

	// Background work
	case requestFailedMsg:
		m, cmd = m.toast(theme.ToneDanger, fmt.Sprintf("Couldn't %s: %v", msg.action, msg.err))
	case aiRewriteResultMsg:
		if msg.session != m.state.chat.session {
			return m, nil, true // started in a room we already left
		}
		m.state.chat.aiEnhancing = false
		if msg.err != nil {
			m, cmd = m.toast(theme.ToneDanger, "AI rewrite failed. Your draft is unchanged.")
			break
		}
		m.state.chat.messageInput.SetValue(msg.rewritten)
		m.state.chat.messageInput.CursorEnd()
		m.state.chat.companion.mood = "fancy words ✧"
	case imageFileSelectedMsg:
		m, cmd = m.uploadImage(msg.path)
	case uploadResultMsg:
		m, cmd = m.uploadResultUpdate(msg)
	case imageFetchedMsg:
		m = m.imageFetchedUpdate(msg)

	default:
		return m, nil, false
	}
	return m, cmd, true
}

func (m model) roomErrorUpdate(msg roomErrorMsg) (model, tea.Cmd) {
	switch msg.code {
	case "AUTH_FAILED", "JOIN_FAILED":
		return m.alert(theme.ToneDanger, "Connection error", msg.message), nil
	case "RATE_LIMITED":
		m, cmd := m.toast(theme.ToneWarning, "Slow down a little: you're sending too fast")
		return m, tea.Batch(cmd, m.waitForRoomEvent())
	}
	return m, m.waitForRoomEvent()
}

func (m model) messageReceivedUpdate(msg messageReceivedMsg) (model, tea.Cmd) {
	c := &m.state.chat
	atBottom := c.viewport.AtBottom()
	c.messages = append(c.messages, msg.message)

	m, fetchCmd := m.fetchImageFor(msg.message.ID, msg.message.Content)

	own := m.isCurrentUser(msg.message.UserID)
	if own {
		m.state.chat.companion.mood = sentMoods[len(m.state.chat.messages)%len(sentMoods)]
	}

	m = m.refreshMessages()
	// Follow the conversation unless the user scrolled up to read history.
	c = &m.state.chat
	if atBottom || own || c.edit.active {
		if !c.edit.active {
			c.viewport.GotoBottom()
		}
		c.unread = 0
	} else {
		c.unread++
	}
	return m, tea.Batch(fetchCmd, m.waitForRoomEvent())
}

// removeMessage drops a message, keeping the edit-mode selection on the same
// message.
func (m model) removeMessage(messageID string) model {
	c := &m.state.chat
	for i, message := range c.messages {
		if message.ID != messageID {
			continue
		}
		c.messages = append(c.messages[:i], c.messages[i+1:]...)
		// Indices after i shift down by one.
		switch sel := c.edit.selected; {
		case sel > i:
			c.edit.selected--
		case sel == i:
			c.edit.selected = -1
			if c.edit.active {
				c.edit.selected = m.lastOwnMessage()
				// Nothing of ours is left to select.
				c.edit.active = c.edit.selected >= 0
			}
		}
		break
	}
	return m
}

func (m model) memberJoinedUpdate(msg memberJoinedMsg) (model, tea.Cmd) {
	if !m.state.chat.members.add(msg.member) || m.isCurrentUser(msg.member.ID) {
		return m, m.waitForRoomEvent()
	}
	m.state.chat.companion.mood = "a new friend!"
	m, cmd := m.toast(theme.ToneSuccess, msg.member.Username+" joined")
	return m, tea.Batch(cmd, m.waitForRoomEvent())
}

func (m model) kickedUpdate(msg kickedMsg) (model, tea.Cmd) {
	if m.isCurrentUser(msg.userID) {
		content := "The room owner removed you."
		if msg.reason != "" {
			content += "\nReason: " + msg.reason
		}
		return m.alert(theme.ToneDanger, "You were kicked", content), m.closeRoomAfter(kickedDelay)
	}

	name := m.state.chat.members.name(msg.userID)
	m.state.chat.members.remove(msg.userID)
	if name == "" {
		return m, m.waitForRoomEvent()
	}
	m, cmd := m.toast(theme.ToneWarning, name+" was kicked")
	return m, tea.Batch(cmd, m.waitForRoomEvent())
}

func (m model) uploadResultUpdate(msg uploadResultMsg) (model, tea.Cmd) {
	if msg.session != m.state.chat.session {
		return m, nil // started in a room we already left
	}
	m.state.chat.uploading = false
	if msg.err != nil {
		return m.toast(theme.ToneDanger, fmt.Sprintf("Upload failed: %v", msg.err))
	}
	if m.state.chat.room == nil {
		return m, nil
	}
	m.state.chat.companion.mood = "cute pic!"
	return m, m.sendContent("send image message", msg.fileURL)
}
