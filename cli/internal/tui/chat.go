package tui

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
	"github.com/hilthontt/visper/cli/internal/strfn"
	"github.com/hilthontt/visper/cli/internal/tui/embeds"
	"github.com/hilthontt/visper/cli/internal/tui/validate"
	"github.com/hilthontt/visper/cli/internal/utils"
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

	// Message editing
	editMode             bool
	selectedMessageIndex int
	editingMessageID     string

	// Member kicking
	selectedKickUserID string

	// Cache for the sidebar image
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

	// AI enhancement
	aiEnhancing    bool
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
}

type roomExpiredMsg struct{}

type roomExpirationDismissedMsg struct{}

type roomExpirationRedirectMsg struct{}

type imageFetchedMsg struct {
	messageID string
	url       string
	bytes     []byte
	err       error
}

func (m model) ChatSwitch(newRoom *apisdk.RoomResponse) (model, tea.Cmd) {
	m = m.stopNotificationWebSocket()

	m = m.SwitchPage(chatPage)
	m.state.chat.room = newRoom

	// Always reset the key so a previous room's key is never reused.
	m.client.Message.SetEncryptionKey(newRoom.EncryptionKey)

	if m.state.chat.roomCode == "" {
		msgInput := textinput.New()
		msgInput.Placeholder = "Type a message..."
		msgInput.Focus()
		msgInput.SetWidth(50)

		searchInput := textinput.New()
		searchInput.Placeholder = "Search participants..."
		searchInput.SetWidth(20)

		editInput := textinput.New()
		editInput.Placeholder = "Edit your message..."
		editInput.SetWidth(50)

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
			searchActive:         false,
			editMode:             false,
			selectedMessageIndex: -1,
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

		m = m.withSidebarImage()

		return m, tea.Batch(
			m.connectWebSocket(newRoom.ID),
			m.startExpirationCountdown(),
		)
	}

	return m, nil
}

func (m model) startExpirationCountdown() tea.Cmd {
	return func() tea.Msg {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-m.state.chat.expirationCtx.Done():
				return nil
			case <-ticker.C:
				remaining := time.Until(m.state.chat.expiresAt)
				if remaining <= 0 {
					return roomExpiredMsg{}
				}
				return roomExpirationTickMsg{remaining: remaining}
			}
		}
	}
}

func (m model) ChatUpdate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case aiEnhanceResultMsg:
		m.state.chat.aiEnhancing = false
		if msg.err != nil {
			m.state.notify = notifyState{
				open:          true,
				title:         "AI Enhancement Failed",
				content:       "Could not reach AI service. Message sent as-is",
				confirmAction: NoAction,
			}
			return m, nil
		}
		m.state.chat.messageInput.SetValue(msg.enhanced)
		return m, nil
	case imageFileSelectedMsg:
		if m.state.chat.room == nil {
			return m, nil
		}
		// Show uploading indicator by reusing the AI enhancing flag concept
		m.state.chat.aiEnhancing = true
		return m, m.uploadFile(msg.path)

	case imageFetchedMsg:
		delete(m.state.chat.imageFetching, msg.messageID)
		if msg.err != nil {
			log.Printf("Failed to fetch image for message %s: %v", msg.messageID, msg.err)
			m.state.chat.imageFailed[msg.messageID] = true
		} else {
			m.state.chat.imageFetched[msg.messageID] = msg.bytes
			m = m.renderImagePreview(msg.messageID)
		}
		m.state.chat.messagesViewport.SetContent(m.renderMessages())
		return m, nil
	case fileUploadResultMsg:
		m.state.chat.aiEnhancing = false
		if msg.err != nil {
			m.state.notify = notifyState{
				open:          true,
				title:         "Upload Failed",
				content:       fmt.Sprintf("Could not upload image: %v", msg.err),
				confirmAction: NoAction,
			}
			return m, nil
		}
		if m.state.chat.room == nil {
			return m, nil
		}

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
		if !m.state.notify.open {
			m.state.notify = notifyState{
				open:          true,
				title:         "Request Failed",
				content:       fmt.Sprintf("Could not %s: %v", msg.action, msg.err),
				confirmAction: NoAction,
			}
		}
		return m, nil
	case roomExpirationTickMsg:
		m.state.chat.timeRemaining = formatDuration(msg.remaining)

		// Show warning at 5 minutes
		if msg.remaining <= 5*time.Minute && msg.remaining > 4*time.Minute+50*time.Second && !m.state.chat.expirationWarned && !m.state.notify.open {
			m.state.chat.expirationWarned = true
			m.state.notify = notifyState{
				open:          true,
				title:         "⚠️  Room Expiring Soon",
				content:       "This room will expire in 5 minutes. All messages will be lost.",
				confirmAction: NoAction,
			}
		}

		return m, m.startExpirationCountdown()

	case roomExpiredMsg:
		if m.state.chat.expirationCancel != nil {
			m.state.chat.expirationCancel()
		}

		m.state.notify = notifyState{
			open:          true,
			title:         "Room Expired",
			content:       "This room has expired. You can continue chatting, but the room will be deleted when all users leave.",
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
		m.state.chat.roomCode = msg.joinCode
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil

	case wsChannelReadyMsg:
		m.state.chat.wsMsgChan = msg.msgChan
		return m, tea.Batch(
			waitForWSMessage(msg.msgChan),
		)

	case wsMessageReceivedMsg:
		m.state.chat.messages = append(m.state.chat.messages, msg.message)

		if id := msg.message.ID; isImageURL(msg.message.Content) &&
			!m.state.chat.imageFailed[id] && !m.state.chat.imageFetching[id] {
			if _, fetched := m.state.chat.imageFetched[id]; !fetched {
				m.state.chat.imageFetching[id] = true
				cmds = append(cmds, fetchImage(id, msg.message.Content))
			}
		}

		m.state.chat.messagesViewport.SetContent(m.renderMessages())
		m.state.chat.messagesViewport.GotoBottom()
		if m.state.chat.wsMsgChan != nil {
			cmds = append(cmds, waitForWSMessage(m.state.chat.wsMsgChan))
		}
		return m, tea.Batch(cmds...)

	case wsMessageUpdatedMsg:
		for i, message := range m.state.chat.messages {
			if message.ID == msg.messageID {
				m.state.chat.messages[i].Content = msg.content
				break
			}
		}
		m.state.chat.messagesViewport.SetContent(m.renderMessages())
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil
	case wsMessageDeletedMsg:
		for i, message := range m.state.chat.messages {
			if message.ID == msg.messageID {
				m.state.chat.messages = append(m.state.chat.messages[:i], m.state.chat.messages[i+1:]...)
				break
			}
		}
		m.state.chat.messagesViewport.SetContent(m.renderMessages())
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil

	case wsMemberJoinedMsg:
		exists := false
		for _, p := range m.state.chat.participants {
			if p.ID == msg.member.ID {
				exists = true
				break
			}
		}

		if !exists {
			m.state.chat.participants = append(m.state.chat.participants, msg.member)
			if !m.state.chat.searchActive || m.state.chat.searchInput.Value() == "" {
				m.state.chat.filteredIndices = append(m.state.chat.filteredIndices, len(m.state.chat.participants)-1)
			}
		}
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil

	case wsMemberLeftMsg:
		for i, p := range m.state.chat.participants {
			if p.ID == msg.userID {
				m.state.chat.participants = append(m.state.chat.participants[:i], m.state.chat.participants[i+1:]...)
				break
			}
		}
		m.state.chat.filteredIndices = make([]int, 0)
		for i := range m.state.chat.participants {
			m.state.chat.filteredIndices = append(m.state.chat.filteredIndices, i)
		}
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil

	case wsMemberListMsg:
		m.state.chat.participants = msg.members
		m.state.chat.filteredIndices = make([]int, len(msg.members))
		for i := range m.state.chat.filteredIndices {
			m.state.chat.filteredIndices[i] = i
		}
		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil

	case roomExpirationRedirectMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case roomExpirationDismissedMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()
	case wsKickTimeoutMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case wsRoomDeletedTimeoutMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case wsKickedMsg:
		// Check if I'm the one being kicked
		if m.userID != nil && *m.userID == msg.userID {
			m.state.notify = notifyState{
				open:          true,
				title:         "Kicked from Room",
				content:       fmt.Sprintf("You were kicked by %s\nReason: %s", msg.username, msg.reason),
				confirmAction: NoAction,
			}
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return wsKickTimeoutMsg{}
			})
		}

		// Someone else was kicked - remove them from participant list
		for i, p := range m.state.chat.participants {
			if p.ID == msg.userID {
				m.state.chat.participants = append(m.state.chat.participants[:i], m.state.chat.participants[i+1:]...)
				break
			}
		}

		m.state.chat.filteredIndices = make([]int, 0)
		for i := range m.state.chat.participants {
			m.state.chat.filteredIndices = append(m.state.chat.filteredIndices, i)
		}

		if m.state.chat.wsMsgChan != nil {
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}
		return m, nil
	case wsRoomDeletedMsg:
		m.state.notify = notifyState{
			open:          true,
			title:         "Room Deleted",
			content:       "This room has been deleted by the owner",
			confirmAction: NoAction,
		}
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return wsRoomDeletedTimeoutMsg{}
		})

	case wsErrorMsg:
		if msg.code == "AUTH_FAILED" || msg.code == "JOIN_FAILED" {
			m.state.notify = notifyState{
				open:          true,
				title:         "Connection Error",
				content:       msg.message,
				confirmAction: NoAction,
			}
			return m, nil
		}

		if msg.code == "RATE_LIMITED" {
			m.state.notify = notifyState{
				open:          true,
				title:         "Rate Limited",
				content:       "You're sending messages too quickly. Please slow down.",
				confirmAction: NoAction,
			}
			return m, waitForWSMessage(m.state.chat.wsMsgChan)
		}

		return m, waitForWSMessage(m.state.chat.wsMsgChan)

	case wsDisconnectedMsg:
		m.state.notify = notifyState{
			open:          true,
			title:         "Disconnected",
			content:       "Lost connection to the chat room",
			confirmAction: NoAction,
		}
		return m, nil

	case participantSearchResultMsg:
		if len(msg.results) == 0 {
			m.state.chat.filteredIndices = make([]int, len(m.state.chat.participants))
			for i := range m.state.chat.filteredIndices {
				m.state.chat.filteredIndices[i] = i
			}
		} else {
			m.state.chat.filteredIndices = make([]int, len(msg.results))
			for i, result := range msg.results {
				m.state.chat.filteredIndices[i] = int(result.HayIndex)
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		centerWidth := m.chatCenterWidth()
		columnHeight := m.chatColumnHeight()

		m.state.chat.messagesViewport.SetWidth(centerWidth)
		m.state.chat.messagesViewport.SetHeight(columnHeight - 4)
		m.state.chat.messageInput.SetWidth(centerWidth - 2)
		m.state.chat.editInput.SetWidth(ModalWidth - 8)

		// Re-render cached images for the new size from the bytes we already
		// have, instead of leaving them stuck on "loading".
		m.state.chat.imagePreviews = make(map[string]string)
		for id := range m.state.chat.imageFetched {
			m = m.renderImagePreview(id)
		}
		m = m.withSidebarImage()
		m.state.chat.messagesViewport.SetContent(m.renderMessages())

	case tea.KeyPressMsg:
		// Handle modal input first if modal is open
		if m.state.notify.open {
			switch m.state.notify.confirmAction {
			case FileExplorerAction:
				m, cmd = m.fileExplorerUpdate(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			case NoAction:
				switch msg.String() {
				case "enter", "esc", "space", "o", "O":
					m = m.closeModal()
					return m, nil
				}
			case RoomExpiredAction:
				switch msg.String() {
				case "enter", "esc", "space", "o", "O", "y", "Y":
					return m, func() tea.Msg {
						return roomExpirationDismissedMsg{}
					}
				}
			case NewJoinCodeAction:
				switch msg.String() {
				case "y", "Y", "enter":
					m = m.closeModal()
					if m.state.chat.room == nil {
						return m, nil
					}
					roomID, client := m.state.chat.room.ID, m.client
					return m, m.chatRequest("generate a new join code", func(ctx context.Context, opts []option.RequestOption) error {
						return client.Room.GenerateNewJoinCode(ctx, roomID, opts...)
					})
				case "n", "N", "esc":
					m = m.closeModal()
					return m, nil
				}
			case ShowQRCodeAction:
				m = m.closeModal()
				return m, nil
			case KickMemberAction:
				switch msg.String() {
				case "y", "Y", "enter":
					return m, func() tea.Msg {
						return kickMemberSubmittedMsg{
							userID: m.state.chat.selectedKickUserID,
						}
					}
				case "n", "N", "esc":
					m = m.closeModal()
					m.state.chat.selectedKickUserID = ""
					return m, nil
				}
			case GoBackAction:
				switch msg.String() {
				case "y", "Y", "enter":
					m = m.closeModal()
					m.clearChatState()
					return m.NewRoomSwitch()
				case "n", "N", "esc":
					m = m.closeModal()
					return m, nil
				}
			case DeleteMessageAction:
				switch msg.String() {
				case "y", "Y", "enter":
					m = m.closeModal()
					m.state.chat.editMode = false
					m.state.chat.selectedMessageIndex = -1
					return m, func() tea.Msg {
						return messageDeleteSubmittedMsg{
							messageID: m.state.chat.editingMessageID,
						}
					}
				case "n", "N", "esc":
					m = m.closeModal()
					return m, nil
				}
			case EditMessageAction:
				m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
				cmds = append(cmds, cmd)

				switch msg.String() {
				case "enter":
					// Submit the edit
					newContent := m.state.chat.editInput.Value()
					originalContent := m.getMessageContent(m.state.chat.editingMessageID)

					if newContent != "" && newContent != originalContent {
						m = m.closeModal()
						return m, func() tea.Msg {
							return messageEditSubmittedMsg{
								messageID:  m.state.chat.editingMessageID,
								newContent: newContent,
							}
						}
					}
					m = m.closeModal()
					return m, nil
				case "esc":
					m = m.closeModal()
					m.state.chat.editMode = false
					m.state.chat.selectedMessageIndex = -1
					return m, nil
				}
				return m, tea.Batch(cmds...)
			}
			// Don't process other keys when modal is open
			return m, nil
		}

		// Handle edit mode navigation
		if m.state.chat.editMode && !m.state.notify.open {
			switch msg.String() {
			case "up", "k":
				m.state.chat.selectedMessageIndex = m.getPreviousOwnMessageIndex(m.state.chat.selectedMessageIndex)
				return m, nil
			case "down", "j":
				m.state.chat.selectedMessageIndex = m.getNextOwnMessageIndex(m.state.chat.selectedMessageIndex)
				return m, nil
			case "delete", "ctrl+d":
				if m.state.chat.selectedMessageIndex >= 0 && m.state.chat.selectedMessageIndex < len(m.state.chat.messages) {
					selectedMsg := m.state.chat.messages[m.state.chat.selectedMessageIndex]
					m.state.chat.editingMessageID = selectedMsg.ID
					m = m.openDeleteMessageModal(selectedMsg.Content)
					return m, nil
				}
				return m, nil
			case "enter":
				if m.state.chat.selectedMessageIndex >= 0 && m.state.chat.selectedMessageIndex < len(m.state.chat.messages) {
					selectedMsg := m.state.chat.messages[m.state.chat.selectedMessageIndex]
					m.state.chat.editingMessageID = selectedMsg.ID
					m.state.chat.editInput.SetValue(selectedMsg.Content)
					m = m.openEditMessageModal(selectedMsg.Content)
					return m, nil
				}
				return m, nil
			case "esc", "ctrl+e":
				m.state.chat.editMode = false
				m.state.chat.selectedMessageIndex = -1
				return m, nil
			}
			return m, nil
		}

		switch {
		case msg.String() == "ctrl+u":
			m = m.openFileExplorer()
			return m, nil
		case msg.String() == "ctrl+p":
			m = m.openQrCodeModal()
			return m, nil
		case msg.String() == "ctrl+a":
			content := m.state.chat.messageInput.Value()
			if content == "" || m.state.chat.aiEnhancing {
				return m, nil
			}
			m.state.chat.aiEnhancing = true
			// TODO: Set default later
			if m.state.chat.aiEnhanceStyle == "" {
				m.state.chat.aiEnhanceStyle = "professional"
			}
			if m.state.chat.aiEnhanceTone == "" {
				m.state.chat.aiEnhanceTone = "polite"
			}
			return m, m.enhanceMessage(content)
		case key.Matches(msg, keys.BackToMenu):
			m = m.openWarnModalForLeaveRoom()
			return m, nil
		case key.Matches(msg, keys.NewJoinCode):
			if !m.state.chat.isRoomOwner {
				m.state.notify = notifyState{
					open:          true,
					title:         "Permission Denied",
					content:       "Only the room owner can generate a new join code",
					confirmAction: NoAction,
				}
				return m, nil
			}

			m = m.openNewJoinCodeModal()

			return m, nil
		case key.Matches(msg, keys.ToggleSearch):
			m.state.chat.searchActive = !m.state.chat.searchActive
			if m.state.chat.searchActive {
				m.state.chat.focusedInput = focusSearch
				m.state.chat.searchInput.Focus()
				m.state.chat.messageInput.Blur()
			} else {
				m.state.chat.focusedInput = focusMessage
				m.state.chat.messageInput.Focus()
				m.state.chat.searchInput.Blur()
				m.state.chat.searchInput.SetValue("")
				m.state.chat.filteredIndices = make([]int, len(m.state.chat.participants))
				for i := range m.state.chat.filteredIndices {
					m.state.chat.filteredIndices[i] = i
				}
			}
			return m, nil
		case msg.String() == "ctrl+e":
			// Toggle edit mode
			if !m.state.chat.editMode {
				// Enter edit mode and select the most recent own message
				m.state.chat.editMode = true
				m.state.chat.selectedMessageIndex = m.getLastOwnMessageIndex()
			} else {
				m.state.chat.editMode = false
				m.state.chat.selectedMessageIndex = -1
			}
		case key.Matches(msg, keys.Enter):
			if m.state.chat.focusedInput == focusSearch && m.state.chat.searchActive && len(m.state.chat.filteredIndices) == 1 && m.state.chat.isRoomOwner {
				participantIdx := m.state.chat.filteredIndices[0]
				if participantIdx >= 0 && participantIdx < len(m.state.chat.participants) {
					selectedParticipant := m.state.chat.participants[participantIdx]

					// Don't allow kicking yourself
					if m.userID != nil && selectedParticipant.ID == *m.userID {
						m.state.notify = notifyState{
							open:          true,
							title:         "Cannot Kick Yourself",
							content:       "You cannot kick yourself from the room",
							confirmAction: NoAction,
						}
						return m, nil
					}

					m.state.chat.selectedKickUserID = selectedParticipant.ID
					m = m.openKickMemberModal(selectedParticipant.Username)
					return m, nil
				}
			}

			if m.state.chat.focusedInput == focusMessage && m.state.chat.messageInput.Value() != "" {
				content := m.state.chat.messageInput.Value()

				limit := validate.WithinLen(1, maxMessageLen, "message")
				limitText := fmt.Sprintf("Messages must be between 1 and %d characters", maxMessageLen)
				if m.state.chat.room != nil && m.state.chat.room.EncryptionKey != "" {
					limit = validate.MaxBytes(maxEncryptedMessageBytes, "message")
					limitText = fmt.Sprintf("Encrypted messages are limited to %d bytes", maxEncryptedMessageBytes)
				}
				validator := validate.Compose(validate.NotEmpty("message"), limit)

				if err := validator(content); err != nil {
					m.state.notify = notifyState{
						open:          true,
						title:         "Invalid Message",
						content:       limitText,
						confirmAction: NoAction,
					}
					return m, nil
				}

				m.state.chat.messageInput.SetValue("")

				if m.state.chat.room == nil {
					return m, nil
				}
				roomID, client := m.state.chat.room.ID, m.client
				return m, m.chatRequest("send message", func(ctx context.Context, opts []option.RequestOption) error {
					_, err := client.Message.Send(ctx, roomID, apisdk.SendMessageParams{Content: content}, opts...)
					return err
				})
			}
		case key.Matches(msg, keys.Back):
			if m.state.chat.searchActive {
				m.state.chat.searchActive = false
				m.state.chat.focusedInput = focusMessage
				m.state.chat.searchInput.Blur()
				m.state.chat.messageInput.Focus()
				m.state.chat.searchInput.SetValue("")
				m.state.chat.filteredIndices = make([]int, len(m.state.chat.participants))
				for i := range m.state.chat.filteredIndices {
					m.state.chat.filteredIndices[i] = i
				}
				return m, nil
			}
		}
	}

	// Update the appropriate input (only if modal is not open)
	if !m.state.notify.open {
		if m.state.notify.confirmAction == EditMessageAction {
			m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
			cmds = append(cmds, cmd)
		}

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
		}
	} else if m.state.notify.confirmAction == EditMessageAction {
		m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
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
	for i := currentIndex - 1; i >= 0; i-- {
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

func (m model) ChatView() string {
	if m.size == undersized || m.size == small {
		return m.chatViewCompact()
	}

	leftWidth := 25
	rightWidth := 40
	centerWidth := m.viewportWidth - leftWidth - rightWidth - 4

	header := m.renderChatHeader()
	headerHeight := lipgloss.Height(header)
	columnHeight := m.viewportHeight - headerHeight

	// Sync viewport dimensions every frame to stay consistent
	m.state.chat.messagesViewport.SetWidth(centerWidth)
	m.state.chat.messagesViewport.SetHeight(columnHeight - 4)
	m.state.chat.messageInput.SetWidth(centerWidth - 2)

	leftColumn := m.renderParticipantsSidebar(leftWidth, columnHeight)
	centerColumn := m.renderChatCenter(centerWidth, columnHeight)
	rightColumn := m.renderRightSidebar(rightWidth, columnHeight)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, centerColumn, rightColumn)
	content := lipgloss.JoinVertical(lipgloss.Left, header, body)

	baseView := m.theme.Base().
		Width(m.viewportWidth).
		Height(m.viewportHeight).
		Render(content)

	if m.state.notify.open {
		var notifyModal string
		switch m.state.notify.confirmAction {
		case EditMessageAction:
			notifyModal = m.RenderEditModal()
		case FileExplorerAction:
			notifyModal = m.RenderFileExplorerModal()
		default:
			notifyModal = m.RenderWarnModal()
		}
		overlayX := (m.viewportWidth - ModalWidth) / 2
		overlayY := (m.viewportHeight - ModalHeight) / 2
		return strfn.PlaceOverlay(overlayX, overlayY, notifyModal, baseView)
	}
	return baseView
}

func (m model) renderChatHeader() string {
	// Room info with expiration timer
	var roomInfo string
	if m.state.chat.timeRemaining != "" {
		timeColor := lipgloss.Color("#10B981") // Green by default
		remaining := time.Until(m.state.chat.expiresAt)

		if remaining <= 5*time.Minute {
			timeColor = lipgloss.Color("#EF4444") // Red for < 5 minutes
		} else if remaining <= 15*time.Minute {
			timeColor = lipgloss.Color("#F59E0B") // Orange for < 15 minutes
		}

		timerStyle := m.theme.Base().Foreground(timeColor).Bold(true)
		roomInfo = fmt.Sprintf("Room: %s | %s",
			m.state.chat.roomCode,
			timerStyle.Render(m.state.chat.timeRemaining))
	} else {
		roomInfo = fmt.Sprintf("Room: %s", m.state.chat.roomCode)
	}

	participantCount := fmt.Sprintf("🍣 %d", len(m.state.chat.participants))

	leftPart := m.theme.TextBrand().Bold(true).Render(roomInfo)
	rightPart := m.theme.TextBody().Render(participantCount)

	spacer := max(m.viewportWidth-lipgloss.Width(leftPart)-lipgloss.Width(rightPart)-4, 0)

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPart,
		strings.Repeat(" ", spacer),
		rightPart,
	)

	return m.theme.Base().
		Width(m.viewportWidth).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Padding(0, 1).
		Render(header)
}

func (m model) renderParticipantsSidebar(width, height int) string {
	sb := strings.Builder{}

	searchStyle := m.theme.Base().
		Width(width-2).
		Padding(0, 1)

	sb.WriteString(searchStyle.Render(m.state.chat.searchInput.View()))
	sb.WriteString("\n")

	titleText := fmt.Sprintf("Participants (%d)", len(m.state.chat.filteredIndices))
	title := m.theme.TextAccent().Bold(true).Render(titleText)
	sb.WriteString(m.theme.Base().Padding(0, 1).Render(title))
	sb.WriteString("\n\n")

	for _, idx := range m.state.chat.filteredIndices {
		if idx >= len(m.state.chat.participants) {
			continue
		}

		p := m.state.chat.participants[idx]
		statusIcon := "●"
		status := m.theme.Base().Foreground(lipgloss.Color("#10B981")).Render(statusIcon)
		username := m.theme.TextBody().Render(p.Username)

		line := lipgloss.JoinHorizontal(lipgloss.Left, status, " ", username)
		sb.WriteString(m.theme.Base().Padding(0, 1).Render(line))
		sb.WriteString("\n")
	}

	content := sb.String()

	return m.theme.Base().
		Width(width).
		Height(height).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Render(content)
}

func (m model) renderChatCenter(width, height int) string {
	sb := strings.Builder{}

	messagesContainer := m.theme.Base().
		Width(width).
		Height(height - 4).
		Render(m.state.chat.messagesViewport.View())

	sb.WriteString(messagesContainer)
	sb.WriteString("\n")

	inputBorder := m.theme.Base().
		Width(width).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Padding(0, 1).
		Render(m.state.chat.messageInput.View())

	sb.WriteString(inputBorder)

	if m.state.chat.aiEnhancing {
		enhancingStyle := m.theme.Base().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
		sb.WriteString(enhancingStyle.Render("  ✦ Enhancing with AI..."))
		sb.WriteString("\n")
	}

	var hint string
	if m.state.chat.editMode {
		hint = m.theme.TextAccent().Bold(true).Render("EDIT MODE: ↑/↓ to select, Enter to edit, Esc to cancel")
	} else {
		hint = m.theme.TextBody().Faint(true).Render(
			"Ctrl+S: search | Ctrl+E: edit | Ctrl+U: upload | Ctrl+A: AI enhance",
		)
	}
	sb.WriteString(m.theme.Base().Padding(0, 1).Render(hint))

	return m.theme.Base().
		Width(width).
		Height(height).
		Render(sb.String())
}

func (m model) renderMessages() string {
	sb := strings.Builder{}

	var userID string
	if m.userID == nil {
		userID = "You"
	} else {
		userID = *m.userID
	}

	for i, msg := range m.state.chat.messages {
		timestamp := m.theme.TextBody().Faint(true).Render(msg.CreatedAt.Format("3:04 PM"))

		var username string
		isOwnMessage := userID == msg.UserID

		if isOwnMessage {
			username = m.theme.TextBrand().Bold(true).Render(msg.Username)
		} else {
			username = m.theme.TextAccent().Bold(true).Render(msg.Username)
		}

		// Add selection indicator if in edit mode
		var selectionIndicator string
		if m.state.chat.editMode && isOwnMessage && i == m.state.chat.selectedMessageIndex {
			selectionIndicator = m.theme.Base().
				Foreground(m.theme.Highlight()).
				Bold(true).
				Render("► ")
		} else {
			selectionIndicator = "  "
		}

		header := lipgloss.JoinHorizontal(lipgloss.Left, selectionIndicator, timestamp, " ", username)
		content := m.renderMessageContent(msg, isOwnMessage)

		msgStyle := m.theme.Base().
			Padding(0, 1).
			MarginBottom(1)

		fullMsg := lipgloss.JoinVertical(lipgloss.Left, header, "  "+content)
		sb.WriteString(msgStyle.Render(fullMsg))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m model) renderMessageContent(msg apisdk.MessageResponse, isOwnMessage bool) string {
	if isImageURL(msg.Content) {
		if m.state.chat.imageFailed[msg.ID] {
			return m.theme.TextBody().Faint(true).Render("[image unavailable]")
		}

		if preview, ok := m.state.chat.imagePreviews[msg.ID]; ok {
			return preview
		}

		return m.theme.TextBody().Faint(true).Render("⏳ loading image...")
	}

	if isOwnMessage {
		return m.theme.TextAccent().Render(msg.Content)
	}
	return m.theme.TextBody().Render(msg.Content)
}

func (m model) renderRightSidebar(width, height int) string {
	textHeight := 2
	imageHeight := height - textHeight - 2

	// View has a value receiver, so the image must be cached from Update (see
	// withSidebarImage); rendering it here on a miss would decode and resize
	// the PNG on every frame.
	imageContent := m.state.chat.cachedImageContent
	if m.state.chat.cachedImageWidth != width-2 || m.state.chat.cachedImageHeight != imageHeight {
		imageContent = m.sidebarImage(width-2, imageHeight)
	}

	waifuText := m.theme.TextAccent().
		Italic(true).
		Render("Your waifu approves")

	textStyle := m.theme.Base().
		Width(width - 2).
		Align(lipgloss.Center)

	centeredText := textStyle.Render(waifuText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		imageContent,
		"",
		centeredText,
	)

	return m.theme.Base().
		Width(width).
		Height(height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Render(content)
}

func (m model) chatViewCompact() string {
	return m.theme.Base().
		Width(m.widthContainer).
		Height(m.heightContainer).
		Render(m.theme.TextBody().Render("Chat view requires a larger terminal window"))
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
	m.state.chat.editInput.Focus()
	m.state.notify = notifyState{
		open:          true,
		title:         "Edit Message",
		content:       "",
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

const (
	chatLeftWidth      = 25
	chatRightWidth     = 40
	imagePreviewHeight = 15
)

func (m model) chatCenterWidth() int {
	return m.viewportWidth - chatLeftWidth - chatRightWidth - 4
}

func (m model) chatColumnHeight() int {
	return m.viewportHeight - lipgloss.Height(m.renderChatHeader())
}

// renderImagePreview renders the fetched image for messageID at the current
// width and caches the result.
func (m model) renderImagePreview(messageID string) model {
	data, ok := m.state.chat.imageFetched[messageID]
	if !ok {
		return m
	}
	width := m.chatCenterWidth() - 4
	if width <= 0 {
		return m
	}
	preview, err := m.imagePreviewer.ImagePreviewFromBytes(data, width, imagePreviewHeight, "")
	if err != nil {
		log.Printf("Failed to render image for message %s: %v", messageID, err)
		m.state.chat.imageFailed[messageID] = true
		return m
	}
	m.state.chat.imagePreviews[messageID] = preview
	return m
}

// sidebarImage renders the selected waifu image at the given size.
func (m model) sidebarImage(width, height int) string {
	img := embeds.WaifuImage
	if m.settingsManager.GetUserConfig().SelectedWaifu == waifu2 {
		img = embeds.Waifu2Image
	}
	content, err := m.imagePreviewer.ImagePreviewFromBytes(img, width, height, "")
	if err != nil {
		log.Printf("Failed to load embedded image: %v", err)
		return m.theme.TextBody().Faint(true).Render("Image unavailable")
	}
	return content
}

// withSidebarImage caches the sidebar image for the current layout; it must
// be called from Update whenever the layout changes.
func (m model) withSidebarImage() model {
	if m.size == undersized || m.size == small || m.viewportWidth == 0 {
		return m
	}
	width := chatRightWidth - 2
	height := m.chatColumnHeight() - 4
	if width <= 0 || height <= 0 {
		return m
	}
	m.state.chat.cachedImageContent = m.sidebarImage(width, height)
	m.state.chat.cachedImageWidth = width
	m.state.chat.cachedImageHeight = height
	return m
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
