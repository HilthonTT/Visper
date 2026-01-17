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
	"github.com/hilthontt/visper/cli/pkg/utils"
	"github.com/reinhrst/fzf-lib"
)

//go:embed waifu.png
var waifuImage []byte

//go:embed waifu2.png
var waifu2Image []byte

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

	// Cache for the sidebar image
	cachedImageContent string
	cachedImageWidth   int
	cachedImageHeight  int

	room      *apisdk.RoomResponse
	wsConn    *apisdk.RoomWebSocket
	wsCtx     context.Context
	wsCancel  context.CancelFunc
	wsMsgChan chan tea.Msg
}

type participantSearchResultMsg struct {
	results []fzf.MatchResult
}

type messageEditSubmittedMsg struct {
	messageID  string
	newContent string
}

func (m model) ChatSwitch(newRoom *apisdk.RoomResponse) (model, tea.Cmd) {
	m = m.SwitchPage(chatPage)
	m.state.chat.room = newRoom

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
			room:                 newRoom,
		}

		return m, m.connectWebSocket(newRoom.ID)
	}

	return m, nil
}

func (m model) ChatUpdate(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case messageEditSubmittedMsg:
		// Handle the edit submission - call your API here
		if m.state.chat.room != nil {
			go func() {
				opts := []option.RequestOption{}
				if m.userID != nil && *m.userID != "" {
					opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
				}

				// TODO: Replace with your actual API call
				// err := m.client.Message.Update(
				// 	m.context,
				// 	m.state.chat.room.ID,
				// 	msg.messageID,
				// 	apisdk.UpdateMessageParams{
				// 		Content: msg.newContent,
				// 	},
				// 	opts...,
				// )
				// if err != nil {
				// 	log.Printf("Failed to update message: %v", err)
				// }

				log.Printf("Edit message %s with content: %s", msg.messageID, msg.newContent)
			}()
		}
		return m, nil

	case wsConnectedMsg:
		m.state.chat.wsConn = msg.conn
		return m, m.listenWebSocket()

	case wsChannelReadyMsg:
		m.state.chat.wsMsgChan = msg.msgChan
		return m, waitForWSMessage(msg.msgChan)

	case wsMessageReceivedMsg:
		m.state.chat.messages = append(m.state.chat.messages, msg.message)
		m.state.chat.messagesViewport.SetContent(m.renderMessages())
		m.state.chat.messagesViewport.GotoBottom()

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

	case wsKickTimeoutMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case wsRoomDeletedTimeoutMsg:
		m = m.closeModal()
		m.clearChatState()
		return m.NewRoomSwitch()

	case wsKickedMsg:
		m.state.notify = notifyState{
			open:          true,
			title:         "Kicked from Room",
			content:       fmt.Sprintf("You were kicked by %s\nReason: %s", msg.username, msg.reason),
			confirmAction: NoAction,
		}
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return wsKickTimeoutMsg{}
		})

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
		centerWidth := m.widthContainer - leftWidth - rightWidth - 4

		messagesHeight := m.heightContainer - 8
		m.state.chat.messagesViewport.Width = centerWidth
		m.state.chat.messagesViewport.Height = messagesHeight
		m.state.chat.messageInput.Width = centerWidth - 2
		m.state.chat.editInput.Width = ModalWidth - 8

		m.state.chat.cachedImageContent = ""

	case tea.KeyMsg:
		// Handle modal input first if modal is open
		if m.state.notify.open {
			switch m.state.notify.confirmAction {
			case NoAction:
				switch msg.String() {
				case "enter", "esc", " ", "o", "O":
					m = m.closeModal()
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
			case EditMessageAction:
				// Update the edit input FIRST before checking for special keys
				m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
				cmds = append(cmds, cmd)

				// Then check for submission/cancellation
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
				// Return early to prevent other key handling
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
		case key.Matches(msg, keys.BackToMenu):
			m = m.openWarnModalForLeaveRoom()
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
			return m, nil
		case key.Matches(msg, keys.Enter):
			if m.state.chat.focusedInput == focusMessage && m.state.chat.messageInput.Value() != "" {
				content := m.state.chat.messageInput.Value()
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
								Content: content,
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

	messagesHeight := m.viewportHeight - 6
	m.state.chat.messagesViewport.Width = centerWidth
	m.state.chat.messagesViewport.Height = messagesHeight
	m.state.chat.messageInput.Width = centerWidth - 2

	m.state.chat.messagesViewport.SetContent(m.renderMessages())

	leftColumn := m.renderParticipantsSidebar(leftWidth, m.viewportHeight-2)
	centerColumn := m.renderChatCenter(centerWidth, m.viewportHeight-2)
	rightColumn := m.renderRightSidebar(rightWidth, m.viewportHeight-2)

	header := m.renderChatHeader()

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftColumn,
		centerColumn,
		rightColumn,
	)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
	)

	baseView := m.theme.Base().
		Width(m.viewportWidth).
		Height(m.viewportHeight).
		Render(content)

	// Show modal if open
	if m.state.notify.open {
		var notifyModal string
		if m.state.notify.confirmAction == EditMessageAction {
			notifyModal = m.RenderEditModal()
		} else {
			notifyModal = m.RenderWarnModal()
		}
		overlayX := (m.viewportWidth - ModalWidth) / 2
		overlayY := (m.viewportHeight - ModalHeight) / 2
		return stringfunction.PlaceOverlay(overlayX, overlayY, notifyModal, baseView)
	}

	return baseView
}

func (m model) renderChatHeader() string {
	roomInfo := fmt.Sprintf("Room: %s", m.state.chat.roomCode)
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

	var hint string
	if m.state.chat.editMode {
		hint = m.theme.TextAccent().Bold(true).Render("EDIT MODE: ↑/↓ to select, Enter to edit, Esc to cancel")
	} else {
		hint = m.theme.TextBody().Faint(true).Render("Ctrl+S: search participants | Ctrl+E: edit messages")
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

		var content string
		if isOwnMessage {
			content = m.theme.TextAccent().Render(msg.Content)
		} else {
			content = m.theme.TextBody().Render(msg.Content)
		}

		msgStyle := m.theme.Base().
			Padding(0, 1).
			MarginBottom(1)

		fullMsg := lipgloss.JoinVertical(lipgloss.Left, header, "  "+content)
		sb.WriteString(msgStyle.Render(fullMsg))
		sb.WriteString("\n")
	}

	return sb.String()
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
			img = waifuImage
		case waifu2:
			img = waifu2Image
		default:
			img = waifuImage
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
