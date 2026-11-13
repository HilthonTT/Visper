package tui

import (
	"context"
	"net/url"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The chat page is split over several files:
//
//	chat.go          state, entering and leaving a room
//	chat_update.go   key handling and user actions
//	chat_events.go   room events and async results
//	chat_members.go  the member list and its search
//	chat_messages.go the message list and its rendering
//	chat_images.go   image messages
//	chat_expiry.go   the expiry countdown
//	chat_view.go     layout and view

type chatState struct {
	// session identifies this visit to the room. Cancelling it (on leaving)
	// stops the WebSocket listener and the countdown, and async results carry
	// it so ones that finish after leaving are dropped.
	session    context.Context
	endSession context.CancelFunc

	room     *apisdk.RoomResponse
	roomCode string
	// roomKey encrypts the room's messages and files. It is created by the
	// room's owner and travels only inside invites.
	roomKey     string
	isRoomOwner bool

	ws     *apisdk.RoomWebSocket
	events chan tea.Msg

	messages     []apisdk.MessageResponse
	viewport     viewport.Model
	messageInput textinput.Model
	editInput    textinput.Model
	// unread counts messages that arrived while scrolled up.
	unread int

	edit         editState
	social       socialState
	members      memberState
	companion    companionPanelState
	expiry       expiryState
	images       imageCache
	fileExplorer fileExplorerState

	// Background work shown in the status line.
	aiEnhancing bool
	uploading   bool
}

// editState is select mode (ctrl+e): picking a message to reply to, react to,
// or, if it is yours, edit or delete.
type editState struct {
	active bool
	// selected is the index in messages of the highlighted message, or -1.
	selected int
	// messageID is the message the edit or delete dialog is for.
	messageID string
}

type companionPanelState struct {
	hidden bool
	mood   string
	// The image is rendered in Update and cached for its size; see
	// withCompanionImage.
	image       string
	imageWidth  int
	imageHeight int
}

// closeConnections stops everything that runs in the background for the room.
func (c chatState) closeConnections() {
	if c.endSession != nil {
		c.endSession()
	}
	if c.ws != nil {
		c.ws.Close()
	}
	if c.expiry.cancel != nil {
		c.expiry.cancel()
	}
}

// enterChat opens the chat page for a room that was just created or joined;
// roomKey is the room's encryption key.
func (m model) enterChat(room *apisdk.RoomResponse, roomKey string) (model, tea.Cmd) {
	if m.page == chatPage && m.state.chat.room != nil {
		// A second create/join finished while we are already in a room (e.g.
		// enter on the join page, tab to New room, enter again). Swapping
		// only the room pointer would send to one room while listening to
		// another, so stay put and leave the extra room on the server.
		isOwner := room.CurrentUser.ID != "" && room.CurrentUser.ID == room.Owner.ID
		leave := leaveRoomFunc(m.client, room.ID, isOwner, m.requestOpts())
		return m, func() tea.Msg { leave(); return nil }
	}

	if roomKey == "" {
		// Without the key we could neither read the room nor write to it
		// without sending plaintext, so don't go in.
		isOwner := room.CurrentUser.ID != "" && room.CurrentUser.ID == room.Owner.ID
		leave := leaveRoomFunc(m.client, room.ID, isOwner, m.requestOpts())
		return m, func() tea.Msg {
			leave()
			return visibleError{message: "That room can't be opened without its key. Ask for the full invite code."}
		}
	}
	if room.QRCodeURL != "" {
		// The key rides in the link's fragment, which browsers never send.
		if u, err := apisdk.WithRoomKeyFragment(room.QRCodeURL, roomKey); err == nil {
			r := *room // copy: don't mutate the caller's room
			r.QRCodeURL = u
			room = &r
		}
	}

	m = m.stopNotificationListener()
	// Never let a previous room's listener or countdown outlive it.
	m = m.clearChat()

	m = m.switchPage(chatPage)
	m.state.menu = menuState{}

	// Always reset the key so a previous room's key is never reused.
	m.client.Message.SetEncryptionKey(roomKey)

	if room.CurrentUser.ID != "" {
		m.userID = room.CurrentUser.ID
	}
	// Sealed into what we encrypt so other members can check who wrote it.
	m.client.Message.SetSender(m.userID)

	messageInput := m.newTextInput("Write a message…")
	messageInput.Focus()

	session, endSession := context.WithCancel(context.Background())
	m.state.chat = chatState{
		session:      session,
		endSession:   endSession,
		room:         room,
		roomCode:     room.JoinCode,
		roomKey:      roomKey,
		isRoomOwner:  room.CurrentUser.ID == room.Owner.ID,
		viewport:     newMessagesViewport(),
		messageInput: messageInput,
		editInput:    m.newTextInput("Edit your message…"),
		edit:         editState{selected: -1},
		social:       newSocialState(m.newTextInput("search messages")),
		members:      newMemberState(m.newTextInput("search"), room.Members),
		companion:    companionPanelState{mood: "your waifu approves ♡"},
		expiry:       newExpiryState(session, room.ExpiresAt),
		images:       newImageCache(),
	}

	m = m.syncChatLayout()
	m = m.withCompanionImage()
	m = m.refreshMessages()

	return m, tea.Batch(m.connectRoom(room.ID), m.startExpirationCountdown())
}

// newMessagesViewport returns the scrollable message list.
func newMessagesViewport() viewport.Model {
	vp := viewport.New(viewport.WithWidth(50), viewport.WithHeight(20))
	// The message input has focus, so the viewport must only react to
	// non-printing keys; the defaults (j/k/f/b/u/d/h/l/space, ctrl+u/d)
	// would scroll while typing and clash with chat shortcuts.
	disabled := key.NewBinding(key.WithDisabled())
	vp.KeyMap = viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("pgdown")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		Up:           key.NewBinding(key.WithKeys("up")),
		Down:         key.NewBinding(key.WithKeys("down")),
		HalfPageUp:   disabled,
		HalfPageDown: disabled,
		Left:         disabled,
		Right:        disabled,
	}
	return vp
}

// clearChat stops the room's background work and forgets it.
func (m model) clearChat() model {
	m.state.chat.closeConnections()
	if m.client != nil {
		m.client.Message.SetEncryptionKey("")
	}
	m.state.chat = chatState{}
	return m
}

// leaveChat asks the server to let us leave the room, then returns to the
// start page.
func (m model) leaveChat() (model, tea.Cmd) {
	leave := m.leaveCurrentRoomFunc()
	m = m.clearChat()
	m, cmd := m.gotoNewRoom()
	return m, tea.Batch(cmd, func() tea.Msg { leave(); return nil })
}

// roomClosedMsg returns to the start page once a room has gone (expired,
// deleted, or we were kicked), after the dialog saying so was on screen for a
// moment. session is that of the room the timer was set for.
type roomClosedMsg struct {
	session context.Context
}

// closeRoomAfter shows the start page after a delay.
func (m model) closeRoomAfter(d time.Duration) tea.Cmd {
	session := m.state.chat.session
	return tea.Tick(d, func(time.Time) tea.Msg { return roomClosedMsg{session: session} })
}

// closeRoom returns to the start page if session is still the current one;
// a timer from a room we already left is ignored.
func (m model) closeRoom(session context.Context) (model, tea.Cmd) {
	if session != m.state.chat.session {
		return m, nil
	}
	m = m.closeDialog()
	m = m.clearChat()
	return m.gotoNewRoom()
}

// withJoinCodeInQRURL points the room's invite QR code at a new join code;
// the URL embeds the code, so after a code change the old QR stops working.
func (m model) withJoinCodeInQRURL(joinCode string) model {
	if m.state.chat.room == nil || m.state.chat.room.QRCodeURL == "" {
		return m
	}
	u, err := url.Parse(m.state.chat.room.QRCodeURL)
	if err != nil {
		return m
	}
	q := u.Query()
	if !q.Has("joinCode") {
		return m
	}
	q.Set("joinCode", joinCode)
	u.RawQuery = q.Encode()

	room := *m.state.chat.room // copy: the pointer may be shared with pending commands
	room.QRCodeURL = u.String()
	room.JoinCode = joinCode
	m.state.chat.room = &room
	return m
}

// encrypted reports whether the room encrypts its messages.
func (c chatState) encrypted() bool {
	return c.room != nil && c.roomKey != ""
}

// inviteCode is the code to share for the room: its join code and its key.
func (c chatState) inviteCode() string {
	code, err := apisdk.FormatInviteCode(c.roomCode, c.roomKey)
	if err != nil {
		return c.roomCode
	}
	return code
}
