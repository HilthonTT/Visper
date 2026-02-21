package tui

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
	stringfunction "github.com/hilthontt/visper/cli/pkg/string_function"
	"github.com/hilthontt/visper/cli/pkg/tui/embeds"
	"github.com/hilthontt/visper/cli/pkg/tui/validate"
	"github.com/hilthontt/visper/cli/pkg/utils"
	"github.com/reinhrst/fzf-lib"
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
	searchCtx        context.Context
	searchCancel     context.CancelFunc

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

type newJoinCodeTimeoutMsg struct{}

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
	if m.state.notification.wsCancel != nil {
		m.state.notification.wsCancel()
		m.state.notification.wsCancel = nil
	}
	if m.state.notification.wsConn != nil {
		m.state.notification.wsConn.Close()
		m.state.notification.wsConn = nil
	}
	m.state.notification.wsMsgChan = nil
	m.state.notification.pendingInvite = nil

	m = m.SwitchPage(chatPage)
	m.state.chat.room = newRoom

	if newRoom.EncryptionKey != "" {
		m.client.Message.SetEncryptionKey(newRoom.EncryptionKey)
	}

	if m.state.chat.roomCode == "" {
		msgInput := textinput.New()
		msgInput.Placeholder = "Type a message..."
		msgInput.Focus()
		msgInput.Width = 50

		searchInput := textinput.New()
		searchInput.Placeholder = "Search participants..."
		searchInput.Width = 20

		editInput := textinput.New()
		editInput.Placeholder = "Edit your message..."
		editInput.Width = 50

		vp := viewport.New(50, 20)

		participants := newRoom.Members
		filteredIndices := make([]int, len(participants))
		for i := range filteredIndices {
			filteredIndices[i] = i
		}

		wsCtx, wsCancel := context.WithCancel(context.Background())
		expirationCtx, expirationCancel := context.WithCancel(context.Background())

		m.userID = &newRoom.CurrentUser.ID
		if m.userID != nil && newRoom != nil {
			m.state.chat.isRoomOwner = m.userID == &newRoom.Owner.ID
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
		log.Printf("AI enhanced result: %q", msg.enhanced)
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
		m.state.chat.imageFetching[msg.messageID] = false
		if msg.err != nil {
			m.state.chat.imageFailed[msg.messageID] = true
		} else {
			// Render once, cache the string result
			centerWidth := m.viewportWidth - 25 - 40 - 4 // match your layout
			previewWidth := centerWidth - 4
			preview, err := m.imagePreviewer.ImagePreviewFromBytes(msg.bytes, previewWidth, 15, "")
			if err != nil {
				m.state.chat.imageFailed[msg.messageID] = true
			} else {
				cacheKey := fmt.Sprintf("%s_%d", msg.messageID, previewWidth)
				m.state.chat.imagePreviews[cacheKey] = preview
			}
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

		// Auto-send the image URL as a message
		fileURL := msg.fileURL
		roomID := m.state.chat.room.ID
		userID := ""
		if m.userID != nil {
			userID = *m.userID
		}

		go func() {
			opts := []option.RequestOption{}
			if userID != "" {
				opts = append(opts, option.WithHeader("X-User-ID", userID))
			}
			_, err := m.client.Message.Send(
				m.context,
				roomID,
				apisdk.SendMessageParams{
					Content:   fileURL,
					Encrypted: true,
				},
				opts...,
			)
			if err != nil {
				log.Printf("Failed to send image message: %v", err)
			}
		}()

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
		if m.state.chat.room != nil {
			go func() {
				opts := []option.RequestOption{}
				if m.userID != nil && *m.userID != "" {
					opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
				}

				_, err := m.client.Room.KickMember(
					m.context,
					m.state.chat.room.ID,
					msg.userID,
					opts...,
				)
				if err != nil {
					log.Printf("Failed to kick member: %v", err)
				}
			}()

			m = m.closeModal()
			return m, nil
		}
	case newJoinCodeTimeoutMsg:
		m = m.closeModal()
		return m, nil
	case messageDeleteSubmittedMsg:
		if m.state.chat.room != nil {
			go func() {
				opts := []option.RequestOption{}
				if m.userID != nil && *m.userID != "" {
					opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
				}

				_, err := m.client.Message.Delete(
					m.context,
					m.state.chat.room.ID,
					msg.messageID,
					opts...,
				)
				if err != nil {
					log.Printf("Failed to delete message: %v", err)
				}
			}()
		}
		return m, nil
	case messageEditSubmittedMsg:
		if m.state.chat.room != nil {
			go func() {
				opts := []option.RequestOption{}
				if m.userID != nil && *m.userID != "" {
					opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
				}

				_, err := m.client.Message.Update(
					m.context,
					m.state.chat.room.ID,
					msg.messageID,
					apisdk.UpdateMessageParams{
						Content: msg.newContent,
					},
					opts...,
				)
				if err != nil {
					log.Printf("Failed to update message: %v", err)
				}

				log.Printf("Edit message %s with content: %s", msg.messageID, msg.newContent)
			}()
		}
		return m, nil

	case wsConnectedMsg:
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

		if isImageURL(msg.message.Content) {
			if !m.state.chat.imageFailed[msg.message.ID] {
				if _, fetched := m.state.chat.imageFetched[msg.message.ID]; !fetched {
					cmds = append(cmds, m.fetchImage(msg.message.ID, msg.message.Content))
				}
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
		leftWidth := 25
		rightWidth := 40
		centerWidth := m.viewportWidth - leftWidth - rightWidth - 4

		// Approximate header height — must match renderChatHeader output
		const headerHeight = 3
		columnHeight := m.viewportHeight - headerHeight
		messagesHeight := columnHeight - 4

		m.state.chat.messagesViewport.Width = centerWidth
		m.state.chat.messagesViewport.Height = messagesHeight
		m.state.chat.messageInput.Width = centerWidth - 2
		m.state.chat.editInput.Width = ModalWidth - 8

		m.state.chat.cachedImageContent = ""
		m.state.chat.imagePreviews = make(map[string]string)

	case tea.KeyMsg:
		// Handle modal input first if modal is open
		if m.state.notify.open {
			switch m.state.notify.confirmAction {
			case FileExplorerAction:
				m, cmd = m.fileExplorerUpdate(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			case NoAction:
				switch msg.String() {
				case "enter", "esc", " ", "o", "O":
					m = m.closeModal()
					return m, nil
				}
			case RoomExpiredAction:
				switch msg.String() {
				case "enter", "esc", " ", "o", "O", "y", "Y":
					return m, func() tea.Msg {
						return roomExpirationDismissedMsg{}
					}
				}
			case NewJoinCodeAction:
				switch msg.String() {
				case "y", "Y", "enter":
					if m.state.chat.room != nil {
						go func() {
							opts := []option.RequestOption{}
							if m.userID != nil && *m.userID != "" {
								opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
							}

							err := m.client.Room.GenerateNewJoinCode(
								m.context,
								m.state.chat.room.ID,
								opts...,
							)
							if err != nil {
								log.Printf("Failed to generate new join code: %v", err)
								return
							}
						}()
					}

					return m, func() tea.Msg {
						return newJoinCodeTimeoutMsg{}
					}
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
				if m.state.chat.searchCancel != nil {
					m.state.chat.searchCancel()
				}
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

				validator := validate.Compose(
					validate.NotEmpty("message"),
					validate.WithinLen(1, 1000, "message"),
				)

				if err := validator(content); err != nil {
					m.state.notify = notifyState{
						open:          true,
						title:         "Invalid Message",
						content:       "Messages must be between 1 and 1000 characters",
						confirmAction: NoAction,
					}
					return m, nil
				}

				m.state.chat.messageInput.SetValue("")

				if m.state.chat.room != nil {
					go func() {
						opts := []option.RequestOption{}
						if m.userID != nil && *m.userID != "" {
							opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
						}

						_, err := m.client.Message.Send(
							m.context,
							m.state.chat.room.ID,
							apisdk.SendMessageParams{
								Content:   content,
								Encrypted: true,
							},
							opts...,
						)
						if err != nil {
							log.Printf("Failed to send message: %v", err)
						}
					}()
				}

				return m, nil
			}
		case key.Matches(msg, keys.Back):
			if m.state.chat.searchActive {
				m.state.chat.searchActive = false
				m.state.chat.focusedInput = focusMessage
				m.state.chat.searchInput.Blur()
				m.state.chat.messageInput.Focus()
				if m.state.chat.searchCancel != nil {
					m.state.chat.searchCancel()
				}
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
				if m.state.chat.searchCancel != nil {
					m.state.chat.searchCancel()
				}
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
	return func() tea.Msg {
		if query == "" {
			return participantSearchResultMsg{results: nil}
		}

		source := make([]string, len(m.state.chat.participants))
		for i, p := range m.state.chat.participants {
			source[i] = p.Username
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		m.state.chat.searchCtx = ctx
		m.state.chat.searchCancel = cancel

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
	m.state.chat.messagesViewport.Width = centerWidth
	m.state.chat.messagesViewport.Height = columnHeight - 4
	m.state.chat.messageInput.Width = centerWidth - 2

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
		return stringfunction.PlaceOverlay(overlayX, overlayY, notifyModal, baseView)
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

		centerWidth := m.viewportWidth - 25 - 40 - 4
		previewWidth := centerWidth - 4
		cacheKey := fmt.Sprintf("%s_%d", msg.ID, previewWidth)

		if preview, ok := m.state.chat.imagePreviews[cacheKey]; ok {
			return preview // cached — zero cost
		}

		// Not yet fetched — trigger fetch if not already in flight
		if !m.state.chat.imageFetching[msg.ID] {
			m.state.chat.imageFetching[msg.ID] = true
			// Can't return a Cmd from here, fetch is triggered from wsMessageReceivedMsg/fetchMissingImages
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

	var imageContent string
	if m.state.chat.cachedImageContent != "" &&
		m.state.chat.cachedImageWidth == width-2 &&
		m.state.chat.cachedImageHeight == imageHeight {
		imageContent = m.state.chat.cachedImageContent
	} else {
		userConfig := m.settingsManager.GetUserConfig()

		var img []byte
		switch userConfig.SelectedWaifu {
		case waifu1:
			img = embeds.WaifuImage
		case waifu2:
			img = embeds.Waifu2Image
		default:
			img = embeds.WaifuImage
		}
		var err error
		imageContent, err = m.imagePreviewer.ImagePreviewFromBytes(
			img,
			width-2,
			imageHeight,
			"",
		)
		if err != nil {
			log.Printf("Failed to load embedded image: %v\n", err)
			imageContent = m.theme.TextBody().Faint(true).Render("Image unavailable")
		}

		m.state.chat.cachedImageContent = imageContent
		m.state.chat.cachedImageWidth = width - 2
		m.state.chat.cachedImageHeight = imageHeight
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

	m.state.chat.roomCode = ""
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

func (m model) fetchImage(messageID, url string) tea.Cmd {
	return func() tea.Msg {
		bytes, err := fetchImageBytes(url)
		return imageFetchedMsg{messageID: messageID, url: url, bytes: bytes, err: err}
	}
}

func (m model) fetchMissingImages() tea.Cmd {
	var cmds []tea.Cmd
	for _, msg := range m.state.chat.messages {
		if isImageURL(msg.Content) {
			if !m.state.chat.imageFailed[msg.ID] {
				if _, fetched := m.state.chat.imageFetched[msg.ID]; !fetched {
					cmds = append(cmds, m.fetchImage(msg.ID, msg.Content))
				}
			}
		}
	}
	return tea.Batch(cmds...)
}
