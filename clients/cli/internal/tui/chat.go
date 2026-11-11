package tui

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	"github.com/hilthontt/visper/clients/cli/internal/tui/validate"
	"github.com/hilthontt/visper/clients/cli/internal/utils"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
	"github.com/reinhrst/fzf-lib"
)

// The API rejects message content longer than maxMessageLen characters. For
// encrypted rooms that limit applies to the base64 ciphertext (24-byte nonce +
// 16-byte MAC + plaintext), which caps the plaintext at 710 bytes.
const (
	maxMessageLen            = 1000
	maxEncryptedMessageBytes = maxMessageLen/4*3 - 24 - 16
)

type chatFocus int

const (
	focusMessage chatFocus = iota
	focusSearch
	focusEdit
)

type chatState struct {
	isRoomOwner      bool
	roomCode         string
	participants     []apisdk.UserResponse
	filteredIndices  []int
	messages         []apisdk.MessageResponse
	messageInput     textinput.Model
	searchInput      textinput.Model
	editInput        textinput.Model
	messagesViewport viewport.Model
	focusedInput     chatFocus
	searchActive     bool
	memberCursor     int // highlighted row of the member search results

	// Message editing
	editMode             bool
	selectedMessageIndex int
	editingMessageID     string

	// Member kicking
	selectedKickUserID string

	// Unread messages that arrived while scrolled up.
	unread int

	// The companion panel on the right.
	hideWaifu          bool
	waifuMood          string
	cachedImageContent string
	cachedImageWidth   int
	cachedImageHeight  int

	// Room expiration
	expiresAt        time.Time
	timeRemaining    string
	expirationWarned bool // Track if we've shown the warning
	expirationCtx    context.Context
	expirationCancel context.CancelFunc

	room      *apisdk.RoomResponse
	wsConn    *apisdk.RoomWebSocket
	wsCtx     context.Context
	wsCancel  context.CancelFunc
	wsMsgChan chan tea.Msg

	fileExplorer fileExplorerState

	// Background work shown in the status line.
	aiEnhancing    bool
	uploading      bool
	aiEnhanceStyle string
	aiEnhanceTone  string

	imageFetched map[string][]byte // messageID -> bytes (not URL, so it's cleared when message leaves)
	imageFailed  map[string]bool

	imagePreviews map[string]string // messageID+dimensions -> rendered preview string
	imageFetching map[string]bool   // messageID -> currently fetching
}

type participantSearchResultMsg struct {
	results []fzf.MatchResult
}

type messageEditSubmittedMsg struct {
	messageID  string
	newContent string
}

type messageDeleteSubmittedMsg struct {
	messageID string
}

type kickMemberSubmittedMsg struct {
	userID string
}

type roomExpirationTickMsg struct {
	remaining time.Duration
	ctx       context.Context
}

type roomExpiredMsg struct {
	ctx context.Context
}

type roomExpirationDismissedMsg struct{}

type roomExpirationRedirectMsg struct{}

type imageFetchedMsg struct {
	messageID string
	url       string
	bytes     []byte
	err       error
}

var sentMoods = []string{
	"nice one~",
	"they'll love that ♡",
	"keep it up!",
	"ooh, spicy",
	"so eloquent ✧",
}

func (m model) newChatInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = placeholder
	styles := ti.Styles()
	for _, s := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
		s.Prompt = m.theme.TextBrand().Bold(true)
		s.Text = m.theme.TextAccent()
		s.Placeholder = m.theme.TextSubtle()
	}
	ti.SetStyles(styles)
	return ti
}

func (m model) ChatSwitch(newRoom *apisdk.RoomResponse) (model, tea.Cmd) {
	m = m.stopNotificationWebSocket()

	m = m.SwitchPage(chatPage)
	m.state.chat.room = newRoom
	m.state.menu = menuState{}

	// Always reset the key so a previous room's key is never reused.
	m.client.Message.SetEncryptionKey(newRoom.EncryptionKey)

	if m.state.chat.roomCode != "" {
		return m, nil
	}

	msgInput := m.newChatInput("Write a message…")
	msgInput.Focus()

	searchInput := m.newChatInput("search")
	searchInput.Prompt = "/ "

	editInput := m.newChatInput("Edit your message…")

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

	participants := newRoom.Members
	filteredIndices := make([]int, len(participants))
	for i := range filteredIndices {
		filteredIndices[i] = i
	}

	wsCtx, wsCancel := context.WithCancel(context.Background())
	expirationCtx, expirationCancel := context.WithCancel(context.Background())

	if newRoom.CurrentUser.ID != "" {
		userID := newRoom.CurrentUser.ID
		m.userID = &userID
	}

	m.state.chat = chatState{
		roomCode:             newRoom.JoinCode,
		participants:         participants,
		filteredIndices:      filteredIndices,
		messages:             []apisdk.MessageResponse{},
		messageInput:         msgInput,
		searchInput:          searchInput,
		editInput:            editInput,
		messagesViewport:     vp,
		focusedInput:         focusMessage,
		selectedMessageIndex: -1,
		waifuMood:            "your waifu approves ♡",
		wsCtx:                wsCtx,
		wsCancel:             wsCancel,
		expirationCtx:        expirationCtx,
		expirationCancel:     expirationCancel,
		expiresAt:            newRoom.ExpiresAt,
		room:                 newRoom,
		isRoomOwner:          newRoom.CurrentUser.ID == newRoom.Owner.ID,
		fileExplorer:         fileExplorerState{},
		imageFetched:         make(map[string][]byte),
		imageFailed:          make(map[string]bool),
		imagePreviews:        make(map[string]string),
		imageFetching:        make(map[string]bool),
	}

	m = m.syncChatLayout()
	m = m.withSidebarImage()
	m = m.refreshMessages()

	return m, tea.Batch(
		m.connectWebSocket(newRoom.ID),
		m.startExpirationCountdown(),
	)
}

func (m model) startExpirationCountdown() tea.Cmd {
	ctx, expiresAt := m.state.chat.expirationCtx, m.state.chat.expiresAt
	return func() tea.Msg {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				remaining := time.Until(expiresAt)
				if remaining <= 0 {
					return roomExpiredMsg{ctx: ctx}
				}
				return roomExpirationTickMsg{remaining: remaining, ctx: ctx}
			}
		}
	}
}

// waitWS keeps listening on the room WebSocket, if it is open.
func (m model) waitWS() tea.Cmd {
	if m.state.chat.wsMsgChan != nil {
		return waitForWSMessage(m.state.chat.wsMsgChan)
	}
	return nil
}

func (m model) resetMemberFilter() model {
	m.state.chat.filteredIndices = make([]int, len(m.state.chat.participants))
	for i := range m.state.chat.filteredIndices {
		m.state.chat.filteredIndices[i] = i
	}
	m.state.chat.memberCursor = min(m.state.chat.memberCursor, max(len(m.state.chat.filteredIndices)-1, 0))
	return m
}

func (m model) closeSearch() model {
	m.state.chat.searchActive = false
	m.state.chat.focusedInput = focusMessage
	m.state.chat.searchInput.Blur()
	m.state.chat.messageInput.Focus()
	m.state.chat.searchInput.SetValue("")
	m.state.chat.memberCursor = 0
	return m.resetMemberFilter()
}

func (m model) participantName(userID string) string {
	for _, p := range m.state.chat.participants {
		if p.ID == userID {
			return p.Username
		}
	}
	return ""
}

func (m model) ChatUpdate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case aiEnhanceResultMsg:
		m.state.chat.aiEnhancing = false
		if msg.err != nil {
			return m.toast(theme.ToneDanger, "AI rewrite failed. Your draft is unchanged.")
		}
		m.state.chat.messageInput.SetValue(msg.enhanced)
		m.state.chat.messageInput.CursorEnd()
		m.state.chat.waifuMood = "fancy words ✧"
		return m, nil
	case imageFileSelectedMsg:
		if m.state.chat.room == nil {
			return m, nil
		}
		m.state.chat.uploading = true
		return m, tea.Batch(m.uploadFile(msg.path), animTick(animFastInterval))

	case imageFetchedMsg:
		delete(m.state.chat.imageFetching, msg.messageID)
		if msg.err != nil {
			log.Printf("Failed to fetch image for message %s: %v", msg.messageID, msg.err)
			m.state.chat.imageFailed[msg.messageID] = true
		} else {
			m.state.chat.imageFetched[msg.messageID] = msg.bytes
			m = m.renderImagePreview(msg.messageID)
		}
		atBottom := m.state.chat.messagesViewport.AtBottom()
		m = m.refreshMessages()
		if atBottom {
			m.state.chat.messagesViewport.GotoBottom()
		}
		return m, nil
	case fileUploadResultMsg:
		m.state.chat.uploading = false
		if msg.err != nil {
			return m.toast(theme.ToneDanger, fmt.Sprintf("Upload failed: %v", msg.err))
		}
		if m.state.chat.room == nil {
			return m, nil
		}
		m.state.chat.waifuMood = "cute pic!"

		// Auto-send the image URL as a message. The SDK encrypts it (and sets
		// the encrypted flag) when the room has a key.
		fileURL := msg.fileURL
		roomID := m.state.chat.room.ID
		client := m.client
		return m, m.chatRequest("send image message", func(ctx context.Context, opts []option.RequestOption) error {
			_, err := client.Message.Send(ctx, roomID, apisdk.SendMessageParams{Content: fileURL}, opts...)
			return err
		})

	case chatRequestFailedMsg:
		return m.toast(theme.ToneDanger, fmt.Sprintf("Couldn't %s: %v", msg.action, msg.err))
	case roomExpirationTickMsg:
		if msg.ctx != m.state.chat.expirationCtx {
			return m, nil // countdown of a room we already left
		}
		m.state.chat.timeRemaining = formatDuration(msg.remaining)

		if msg.remaining <= 5*time.Minute && !m.state.chat.expirationWarned {
			m.state.chat.expirationWarned = true
			m.state.chat.waifuMood = "hurry, time's almost up!"
			m, cmd = m.toast(theme.ToneWarning, "This room expires in 5 minutes. Messages will be lost.")
			return m, tea.Batch(cmd, m.startExpirationCountdown())
		}

		return m, m.startExpirationCountdown()

	case roomExpiredMsg:
		if msg.ctx != m.state.chat.expirationCtx {
			return m, nil
		}
		if m.state.chat.expirationCancel != nil {
			m.state.chat.expirationCancel()
		}

		m.state.notify = notifyState{
			open:          true,
			title:         "This room has expired",
			content:       "Its messages are gone. Taking you back to the start…",
			confirmAction: RoomExpiredAction,
		}
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return roomExpirationRedirectMsg{}
		})

	case kickMemberSubmittedMsg:
		m = m.closeModal()
		if m.state.chat.room == nil {
			return m, nil
		}
		roomID, userID, client := m.state.chat.room.ID, msg.userID, m.client
		return m, m.chatRequest("kick member", func(ctx context.Context, opts []option.RequestOption) error {
			_, err := client.Room.KickMember(ctx, roomID, userID, opts...)
			return err
		})
	case messageDeleteSubmittedMsg:
		if m.state.chat.room == nil {
			return m, nil
		}
		roomID, messageID, client := m.state.chat.room.ID, msg.messageID, m.client
		return m, m.chatRequest("delete message", func(ctx context.Context, opts []option.RequestOption) error {
			_, err := client.Message.Delete(ctx, roomID, messageID, opts...)
			return err
		})
	case messageEditSubmittedMsg:
		if m.state.chat.room == nil {
			return m, nil
		}
		roomID, messageID, content, client := m.state.chat.room.ID, msg.messageID, msg.newContent, m.client
		return m, m.chatRequest("update message", func(ctx context.Context, opts []option.RequestOption) error {
			_, err := client.Message.Update(ctx, roomID, messageID, apisdk.UpdateMessageParams{Content: content}, opts...)
			return err
		})

	case wsConnectedMsg:
		if msg.ctx != m.state.chat.wsCtx || msg.ctx.Err() != nil {
			// We left the room while the connection was being established.
			msg.conn.Close()
			return m, nil
		}
		m.state.chat.wsConn = msg.conn
		return m, m.listenWebSocket()

	case wsRoomUpdatedMsg:
		changed := m.state.chat.roomCode != "" && m.state.chat.roomCode != msg.joinCode
		m.state.chat.roomCode = msg.joinCode
		if changed {
			m, cmd = m.toast(theme.ToneSuccess, "New join code: "+msg.joinCode)
			return m, tea.Batch(cmd, m.waitWS())
		}
		return m, m.waitWS()

	case wsChannelReadyMsg:
		if msg.ctx != m.state.chat.wsCtx {
			return m, nil // listener of a room we already left
		}
		m.state.chat.wsMsgChan = msg.msgChan
		return m, waitForWSMessage(msg.msgChan)

	case wsMessageReceivedMsg:
		atBottom := m.state.chat.messagesViewport.AtBottom()
		m.state.chat.messages = append(m.state.chat.messages, msg.message)

		if id := msg.message.ID; isImageURL(msg.message.Content) &&
			!m.state.chat.imageFailed[id] && !m.state.chat.imageFetching[id] {
			if _, fetched := m.state.chat.imageFetched[id]; !fetched {
				m.state.chat.imageFetching[id] = true
				cmds = append(cmds, fetchImage(id, msg.message.Content))
			}
		}

		own := m.userID != nil && msg.message.UserID == *m.userID
		if own {
			m.state.chat.waifuMood = sentMoods[len(m.state.chat.messages)%len(sentMoods)]
		}

		m = m.refreshMessages()
		// Follow the conversation unless the user scrolled up to read history.
		if atBottom || own || m.state.chat.editMode {
			if !m.state.chat.editMode {
				m.state.chat.messagesViewport.GotoBottom()
			}
			m.state.chat.unread = 0
		} else {
			m.state.chat.unread++
		}
		cmds = append(cmds, m.waitWS())
		return m, tea.Batch(cmds...)

	case wsMessageUpdatedMsg:
		for i, message := range m.state.chat.messages {
			if message.ID == msg.messageID {
				m.state.chat.messages[i].Content = msg.content
				break
			}
		}
		m = m.refreshMessages()
		return m, m.waitWS()
	case wsMessageDeletedMsg:
		for i, message := range m.state.chat.messages {
			if message.ID == msg.messageID {
				m.state.chat.messages = append(m.state.chat.messages[:i], m.state.chat.messages[i+1:]...)
				// Keep the edit-mode selection on the same message: indices after
				// i shift down by one.
				switch sel := m.state.chat.selectedMessageIndex; {
				case sel > i:
					m.state.chat.selectedMessageIndex--
				case sel == i:
					m.state.chat.selectedMessageIndex = -1
					if m.state.chat.editMode {
						m.state.chat.selectedMessageIndex = m.getLastOwnMessageIndex()
					}
				}
				break
			}
		}
		m = m.refreshMessages()
		return m, m.waitWS()

	case wsMemberJoinedMsg:
		exists := false
		for _, p := range m.state.chat.participants {
			if p.ID == msg.member.ID {
				exists = true
				break
			}
		}

		if exists {
			return m, m.waitWS()
		}
		m.state.chat.participants = append(m.state.chat.participants, msg.member)
		if !m.state.chat.searchActive || m.state.chat.searchInput.Value() == "" {
			m.state.chat.filteredIndices = append(m.state.chat.filteredIndices, len(m.state.chat.participants)-1)
		}
		if m.userID != nil && msg.member.ID == *m.userID {
			return m, m.waitWS()
		}
		m.state.chat.waifuMood = "a new friend!"
		m, cmd = m.toast(theme.ToneSuccess, msg.member.Username+" joined")
		return m, tea.Batch(cmd, m.waitWS())

	case wsMemberLeftMsg:
		name := msg.username
		if name == "" {
			name = m.participantName(msg.userID)
		}
		for i, p := range m.state.chat.participants {
			if p.ID == msg.userID {
				m.state.chat.participants = append(m.state.chat.participants[:i], m.state.chat.participants[i+1:]...)
				break
			}
		}
		m = m.resetMemberFilter()
		if name == "" {
			return m, m.waitWS()
		}
		m.state.chat.waifuMood = "bye bye~"
		m, cmd = m.toast(theme.ToneInfo, name+" left")
		return m, tea.Batch(cmd, m.waitWS())

	case wsMemberListMsg:
		m.state.chat.participants = msg.members
		m = m.resetMemberFilter()
		return m, m.waitWS()

	case roomExpirationRedirectMsg, roomExpirationDismissedMsg, wsKickTimeoutMsg, wsRoomDeletedTimeoutMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case wsKickedMsg:
		// Check if I'm the one being kicked
		if m.userID != nil && *m.userID == msg.userID {
			content := "The room owner removed you."
			if msg.reason != "" {
				content += "\nReason: " + msg.reason
			}
			m = m.alert(theme.ToneDanger, "You were kicked", content)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return wsKickTimeoutMsg{}
			})
		}

		name := m.participantName(msg.userID)
		for i, p := range m.state.chat.participants {
			if p.ID == msg.userID {
				m.state.chat.participants = append(m.state.chat.participants[:i], m.state.chat.participants[i+1:]...)
				break
			}
		}
		m = m.resetMemberFilter()
		if name == "" {
			return m, m.waitWS()
		}
		m, cmd = m.toast(theme.ToneWarning, name+" was kicked")
		return m, tea.Batch(cmd, m.waitWS())
	case wsRoomDeletedMsg:
		m = m.alert(theme.ToneDanger, "Room deleted", "The owner deleted this room. Taking you back to the start…")
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return wsRoomDeletedTimeoutMsg{}
		})

	case wsErrorMsg:
		switch msg.code {
		case "AUTH_FAILED", "JOIN_FAILED":
			return m.alert(theme.ToneDanger, "Connection error", msg.message), nil
		case "RATE_LIMITED":
			m, cmd = m.toast(theme.ToneWarning, "Slow down a little: you're sending too fast")
			return m, tea.Batch(cmd, m.waitWS())
		}
		return m, m.waitWS()

	case wsDisconnectedMsg:
		return m.alert(theme.ToneDanger, "Disconnected", "Lost the connection to this room. Press ctrl+b to leave."), nil

	case participantSearchResultMsg:
		if len(msg.results) == 0 && m.state.chat.searchInput.Value() == "" {
			m = m.resetMemberFilter()
		} else {
			m.state.chat.filteredIndices = make([]int, len(msg.results))
			for i, result := range msg.results {
				m.state.chat.filteredIndices[i] = int(result.HayIndex)
			}
		}
		m.state.chat.memberCursor = 0
		return m, nil

	case tea.WindowSizeMsg:
		m = m.relayoutChat()

	case tea.KeyPressMsg:
		// Handle modal input first if modal is open
		if m.state.notify.open {
			return m.chatModalUpdate(msg)
		}

		if m.state.chat.editMode {
			return m.chatEditModeUpdate(msg)
		}

		if m.state.chat.searchActive {
			switch msg.String() {
			case "up":
				m.state.chat.memberCursor = max(m.state.chat.memberCursor-1, 0)
				return m, nil
			case "down":
				m.state.chat.memberCursor = min(m.state.chat.memberCursor+1, max(len(m.state.chat.filteredIndices)-1, 0))
				return m, nil
			case "enter":
				return m.kickSelectedMember()
			case "esc":
				return m.closeSearch(), nil
			}
		}

		switch {
		case msg.String() == "ctrl+u":
			return m.openFileExplorer(), nil
		case msg.String() == "ctrl+p":
			return m.openQrCodeModal(), nil
		case msg.String() == "ctrl+w":
			m.state.chat.hideWaifu = !m.state.chat.hideWaifu
			return m.relayoutChat(), nil
		case msg.String() == "ctrl+a":
			content := m.state.chat.messageInput.Value()
			if content == "" {
				return m.toast(theme.ToneInfo, "Write a draft first, then ctrl+a rewrites it")
			}
			if m.state.chat.aiEnhancing {
				return m, nil
			}
			m.state.chat.aiEnhancing = true
			if m.state.chat.aiEnhanceStyle == "" {
				m.state.chat.aiEnhanceStyle = "professional"
			}
			if m.state.chat.aiEnhanceTone == "" {
				m.state.chat.aiEnhanceTone = "polite"
			}
			return m, tea.Batch(m.enhanceMessage(content), animTick(animFastInterval))
		case key.Matches(msg, keys.BackToMenu):
			return m.openWarnModalForLeaveRoom(), nil
		case key.Matches(msg, keys.NewJoinCode):
			if !m.state.chat.isRoomOwner {
				return m.toast(theme.ToneWarning, "Only the room owner can change the join code")
			}
			return m.openNewJoinCodeModal(), nil
		case key.Matches(msg, keys.ToggleSearch):
			if m.state.chat.searchActive {
				return m.closeSearch(), nil
			}
			if m.chatLeftWidth() == 0 {
				return m.toast(theme.ToneInfo, "Widen the window to see the member list")
			}
			m.state.chat.searchActive = true
			m.state.chat.focusedInput = focusSearch
			m.state.chat.memberCursor = 0
			m.state.chat.searchInput.Focus()
			m.state.chat.messageInput.Blur()
			return m, nil
		case msg.String() == "ctrl+e":
			m.state.chat.selectedMessageIndex = m.getLastOwnMessageIndex()
			if m.state.chat.selectedMessageIndex < 0 {
				return m.toast(theme.ToneInfo, "You haven't sent any messages yet")
			}
			m.state.chat.editMode = true
			return m.refreshMessages(), nil
		case key.Matches(msg, keys.Enter) && m.state.chat.focusedInput == focusMessage:
			return m.sendMessage()
		case msg.String() == "end":
			m.state.chat.messagesViewport.GotoBottom()
			m.state.chat.unread = 0
			return m, nil
		}
	}

	// Update the focused input.
	if !m.state.notify.open {
		switch m.state.chat.focusedInput {
		case focusMessage:
			m.state.chat.messageInput, cmd = m.state.chat.messageInput.Update(msg)
			cmds = append(cmds, cmd)
		case focusSearch:
			oldValue := m.state.chat.searchInput.Value()
			m.state.chat.searchInput, cmd = m.state.chat.searchInput.Update(msg)
			cmds = append(cmds, cmd)

			newValue := m.state.chat.searchInput.Value()
			if oldValue != newValue {
				cmds = append(cmds, m.searchParticipants(newValue))
			}
		}

		if !m.state.chat.editMode {
			m.state.chat.messagesViewport, cmd = m.state.chat.messagesViewport.Update(msg)
			cmds = append(cmds, cmd)
			if m.state.chat.messagesViewport.AtBottom() {
				m.state.chat.unread = 0
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) sendMessage() (model, tea.Cmd) {
	content := m.state.chat.messageInput.Value()
	if content == "" {
		return m, nil
	}

	limit := validate.WithinLen(1, maxMessageLen, "message")
	limitText := fmt.Sprintf("Messages can be up to %d characters", maxMessageLen)
	if m.state.chat.room != nil && m.state.chat.room.EncryptionKey != "" {
		limit = validate.MaxBytes(maxEncryptedMessageBytes, "message")
		limitText = fmt.Sprintf("Encrypted messages can be up to %d bytes", maxEncryptedMessageBytes)
	}
	if err := validate.Compose(validate.NotEmpty("message"), limit)(content); err != nil {
		return m.toast(theme.ToneWarning, limitText)
	}

	m.state.chat.messageInput.SetValue("")
	m.state.chat.messagesViewport.GotoBottom()
	m.state.chat.unread = 0

	if m.state.chat.room == nil {
		return m, nil
	}
	roomID, client := m.state.chat.room.ID, m.client
	return m, m.chatRequest("send message", func(ctx context.Context, opts []option.RequestOption) error {
		_, err := client.Message.Send(ctx, roomID, apisdk.SendMessageParams{Content: content}, opts...)
		return err
	})
}

func (m model) kickSelectedMember() (model, tea.Cmd) {
	c := m.state.chat
	if len(c.filteredIndices) == 0 {
		return m, nil
	}
	if !c.isRoomOwner {
		return m.toast(theme.ToneInfo, "Only the room owner can kick members")
	}
	idx := c.filteredIndices[min(c.memberCursor, len(c.filteredIndices)-1)]
	if idx < 0 || idx >= len(c.participants) {
		return m, nil
	}
	selected := c.participants[idx]
	if m.userID != nil && selected.ID == *m.userID {
		return m.toast(theme.ToneInfo, "You can't kick yourself. Use ctrl+b to leave.")
	}
	m.state.chat.selectedKickUserID = selected.ID
	return m.openKickMemberModal(selected.Username), nil
}

// chatModalUpdate handles keys while a dialog is open on the chat page.
func (m model) chatModalUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.state.notify.confirmAction {
	case FileExplorerAction:
		return m.fileExplorerUpdate(msg)
	case NoAction:
		switch msg.String() {
		case "enter", "esc", "space", "o", "O":
			return m.closeModal(), nil
		}
		return m, nil
	case RoomExpiredAction:
		switch msg.String() {
		case "enter", "esc", "space", "o", "O", "y", "Y":
			return m, func() tea.Msg { return roomExpirationDismissedMsg{} }
		}
		return m, nil
	case ShowQRCodeAction:
		return m.closeModal(), nil
	case EditMessageAction:
		switch msg.String() {
		case "enter":
			newContent := m.state.chat.editInput.Value()
			originalContent := m.getMessageContent(m.state.chat.editingMessageID)
			m = m.closeModal()
			if newContent != "" && newContent != originalContent {
				id := m.state.chat.editingMessageID
				return m, func() tea.Msg {
					return messageEditSubmittedMsg{messageID: id, newContent: newContent}
				}
			}
			return m, nil
		case "esc":
			m = m.closeModal()
			m.state.chat.editMode = false
			m.state.chat.selectedMessageIndex = -1
			return m.refreshMessages(), nil
		}
		m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
		return m, cmd
	}

	// Confirm dialogs.
	answer := m.modalAnswer(msg)
	if answer == answerToggle {
		m.state.notify.focus = 1 - m.state.notify.focus
		return m, nil
	}
	if answer == answerNone {
		return m, nil
	}
	yes := answer == answerYes

	switch m.state.notify.confirmAction {
	case NewJoinCodeAction:
		m = m.closeModal()
		if !yes || m.state.chat.room == nil {
			return m, nil
		}
		roomID, client := m.state.chat.room.ID, m.client
		return m, m.chatRequest("generate a new join code", func(ctx context.Context, opts []option.RequestOption) error {
			return client.Room.GenerateNewJoinCode(ctx, roomID, opts...)
		})
	case KickMemberAction:
		if !yes {
			m.state.chat.selectedKickUserID = ""
			return m.closeModal(), nil
		}
		userID := m.state.chat.selectedKickUserID
		return m, func() tea.Msg { return kickMemberSubmittedMsg{userID: userID} }
	case GoBackAction:
		m = m.closeModal()
		if !yes {
			return m, nil
		}
		leave := m.leaveRoomRequest()
		m.clearChatState()
		m, cmd = m.NewRoomSwitch()
		return m, tea.Batch(cmd, func() tea.Msg { leave(); return nil })
	case DeleteMessageAction:
		m = m.closeModal()
		if !yes {
			return m, nil
		}
		m.state.chat.editMode = false
		m.state.chat.selectedMessageIndex = -1
		m = m.refreshMessages()
		id := m.state.chat.editingMessageID
		return m, func() tea.Msg { return messageDeleteSubmittedMsg{messageID: id} }
	}
	return m, nil
}

// chatEditModeUpdate handles keys while picking one of your messages.
func (m model) chatEditModeUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	c := m.state.chat
	valid := c.selectedMessageIndex >= 0 && c.selectedMessageIndex < len(c.messages)

	switch msg.String() {
	case "up", "k":
		m.state.chat.selectedMessageIndex = m.getPreviousOwnMessageIndex(c.selectedMessageIndex)
		return m.refreshMessages(), nil
	case "down", "j":
		m.state.chat.selectedMessageIndex = m.getNextOwnMessageIndex(c.selectedMessageIndex)
		return m.refreshMessages(), nil
	case "delete", "ctrl+d":
		if valid {
			selected := c.messages[c.selectedMessageIndex]
			m.state.chat.editingMessageID = selected.ID
			return m.openDeleteMessageModal(selected.Content), nil
		}
	case "enter":
		if valid {
			selected := c.messages[c.selectedMessageIndex]
			m.state.chat.editingMessageID = selected.ID
			return m.openEditMessageModal(selected.Content), nil
		}
	case "esc", "ctrl+e":
		m.state.chat.editMode = false
		m.state.chat.selectedMessageIndex = -1
		return m.refreshMessages(), nil
	}
	return m, nil
}

// relayoutChat resizes everything that depends on the window size.
func (m model) relayoutChat() model {
	m = m.syncChatLayout()

	// Re-render cached images for the new size from the bytes we already
	// have, instead of leaving them stuck on "loading".
	m.state.chat.imagePreviews = make(map[string]string)
	for id := range m.state.chat.imageFetched {
		m = m.renderImagePreview(id)
	}
	m = m.withSidebarImage()
	return m.refreshMessages()
}

// Helper functions for message navigation
func (m model) getLastOwnMessageIndex() int {
	if m.userID == nil {
		return -1
	}
	userID := *m.userID
	for i := len(m.state.chat.messages) - 1; i >= 0; i-- {
		if m.state.chat.messages[i].UserID == userID {
			return i
		}
	}
	return -1
}

func (m model) getNextOwnMessageIndex(currentIndex int) int {
	if m.userID == nil {
		return currentIndex
	}
	userID := *m.userID
	for i := currentIndex + 1; i < len(m.state.chat.messages); i++ {
		if m.state.chat.messages[i].UserID == userID {
			return i
		}
	}
	return currentIndex
}

func (m model) getPreviousOwnMessageIndex(currentIndex int) int {
	if m.userID == nil {
		return currentIndex
	}
	userID := *m.userID
	for i := min(currentIndex, len(m.state.chat.messages)) - 1; i >= 0; i-- {
		if m.state.chat.messages[i].UserID == userID {
			return i
		}
	}
	return currentIndex
}

func (m model) getMessageContent(messageID string) string {
	for _, msg := range m.state.chat.messages {
		if msg.ID == messageID {
			return msg.Content
		}
	}
	return ""
}

func (m model) searchParticipants(query string) tea.Cmd {
	// Snapshot the usernames now; the command runs on another goroutine.
	source := make([]string, len(m.state.chat.participants))
	for i, p := range m.state.chat.participants {
		source[i] = p.Username
	}
	return func() tea.Msg {
		if query == "" {
			return participantSearchResultMsg{results: nil}
		}

		results, err := utils.FzfSearch(query, source, 500*time.Millisecond)
		if err != nil {
			return participantSearchResultMsg{results: nil}
		}

		return participantSearchResultMsg{results: results}
	}
}

func (m *model) clearChatState() {
	if m.state.chat.wsCancel != nil {
		m.state.chat.wsCancel()
	}

	if m.state.chat.wsConn != nil {
		m.state.chat.wsConn.Close()
	}

	if m.state.chat.expirationCancel != nil {
		m.state.chat.expirationCancel()
	}

	if m.client != nil {
		m.client.Message.SetEncryptionKey("")
	}

	m.state.chat = chatState{}
}

func (m model) openEditMessageModal(currentContent string) model {
	m.state.chat.editInput.SetValue(currentContent)
	m.state.chat.editInput.CursorEnd()
	m.state.chat.editInput.Focus()
	m.state.notify = notifyState{
		open:          true,
		title:         "Edit message",
		confirmAction: EditMessageAction,
	}
	return m
}

func fetchImage(messageID, url string) tea.Cmd {
	return func() tea.Msg {
		bytes, err := fetchImageBytes(url)
		return imageFetchedMsg{messageID: messageID, url: url, bytes: bytes, err: err}
	}
}

// chatRequestFailedMsg reports a failed background API call made from the
// chat page.
type chatRequestFailedMsg struct {
	action string
	err    error
}

// requestOpts returns the per-request options identifying the current user.
func (m model) requestOpts() []option.RequestOption {
	if m.userID != nil && *m.userID != "" {
		return []option.RequestOption{option.WithHeader("X-User-ID", *m.userID)}
	}
	return nil
}

// chatRequest runs fn as a tea.Cmd (off the Update loop) and reports a
// failure as a chatRequestFailedMsg. Everything fn needs must be captured by
// the caller up front; it must not touch the model.
func (m model) chatRequest(action string, fn func(ctx context.Context, opts []option.RequestOption) error) tea.Cmd {
	ctx := m.context
	opts := m.requestOpts()
	return func() tea.Msg {
		if err := fn(ctx, opts); err != nil {
			log.Printf("Failed to %s: %v", action, err)
			return chatRequestFailedMsg{action: action, err: err}
		}
		return nil
	}
}
