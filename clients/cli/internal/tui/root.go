package tui

import (
	"context"
	"fmt"
	"log"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"
	"github.com/hilthontt/visper/clients/cli/internal/preview"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

type page = int
type size = int

const (
	splashPage page = iota
	faqPage
	newRoomPage
	joinRoomPage
	chatPage
	settingsPage
)

const (
	undersized size = iota
	small
	medium
	large
)

type state struct {
	splash       splashState
	cursor       cursorState
	footer       footerState
	menu         menuState
	joinRoom     joinRoomState
	newRoom      newRoomState
	chat         chatState
	settings     settingsState
	faq          faqState
	notify       notifyState
	notification notificationListenerState
	toast        toastState
}

type notificationListenerState struct {
	wsConn        *apisdk.NotificationWebSocket
	wsCtx         context.Context
	wsCancel      context.CancelFunc
	wsMsgChan     chan tea.Msg
	pendingInvite *roomInviteData
}

type roomInviteData struct {
	roomID     string
	joinCode   string
	secureCode string
	timestamp  int64
	expiresAt  time.Time
}

type visibleError struct {
	message string
}

type cleanupCompleteMsg struct{}

type roomJoinedMsg struct {
	room *apisdk.RoomResponse
}

type model struct {
	switched        bool
	page            page
	state           state
	context         context.Context
	client          *apisdk.Client
	error           *visibleError
	viewportWidth   int
	viewportHeight  int
	widthContainer  int
	heightContainer int
	widthContent    int
	heightContent   int
	size            size
	theme           theme.Theme
	faqs            []FAQ
	waifus          []WaifuOption
	imagePreviewer  *preview.ImagePreviewer
	settingsManager settings.Manager
	userID          *string
}

func NewModel() (tea.Model, error) {
	ctx := context.Background()

	userID := uuid.NewString()

	// TODO Display in header
	log.Printf("USERID: %s\n", userID)

	m := model{
		context: ctx,
		page:    splashPage,
		state: state{
			splash: splashState{},
			footer: footerState{
				commands: []footerCommand{},
			},
			menu:     menuState{},
			joinRoom: joinRoomState{},
			newRoom:  newRoomState{},
			chat:     chatState{},
			settings: settingsState{},
			notify:   notifyState{},
		},
		theme:           theme.BasicTheme(true, nil),
		faqs:            LoadFaqs(),
		waifus:          LoadWaifus(),
		imagePreviewer:  preview.NewImagePreviewer(),
		settingsManager: settings.NewManager(),
		userID:          &userID,
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return m.SplashInit()
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}
	m.switched = false

	if env, ok := msg.(wsChanMsg); ok {
		if m.page != chatPage || env.ch != m.state.chat.wsMsgChan {
			return m, nil // stale message from a room we already left
		}
		msg = env.msg
	}

	switch msg := msg.(type) {
	case roomJoinedMsg:
		return m.ChatSwitch(msg.room)
	case notificationWSConnectedMsg:
		if msg.ctx != m.state.notification.wsCtx || msg.ctx.Err() != nil {
			// The listener was cancelled (e.g. we entered a chat) while dialing.
			msg.conn.Close()
			return m, nil
		}
		m.state.notification.wsConn = msg.conn
		return m, m.listenNotificationWebSocket()

	case notificationWSConnectFailedMsg:
		if msg.ctx == m.state.notification.wsCtx {
			m = m.stopNotificationWebSocket()
		}
		return m, nil

	case notificationWSChannelReadyMsg:
		m.state.notification.wsMsgChan = msg.msgChan
		return m, waitForNotificationWSMessage(msg.msgChan)
	case notificationWSRoomInviteMsg:
		if m.page != chatPage {
			m.state.notification.pendingInvite = &roomInviteData{
				roomID:     msg.roomID,
				joinCode:   msg.joinCode,
				secureCode: msg.secureCode,
				timestamp:  msg.timestamp,
				expiresAt:  msg.expiresAt,
			}
			m = m.openRoomInviteModal(msg.roomID)
		}

		if m.state.notification.wsMsgChan != nil {
			return m, waitForNotificationWSMessage(m.state.notification.wsMsgChan)
		}
		return m, nil

	case roomInviteAcceptedMsg:
		if m.state.notification.pendingInvite != nil {
			invite := m.state.notification.pendingInvite
			m.state.notification.pendingInvite = nil
			m = m.closeModal()

			return m, m.joinRoomFromInvite(invite)
		}
		m = m.closeModal()
		return m, nil

	case roomInviteDeclinedMsg:
		m.state.notification.pendingInvite = nil
		m = m.closeModal()

		if m.state.notification.wsMsgChan != nil {
			return m, waitForNotificationWSMessage(m.state.notification.wsMsgChan)
		}
		return m, nil

	case notificationWSErrorMsg:
		log.Printf("Notification WS error: %s - %s", msg.code, msg.message)
		if m.state.notification.wsMsgChan != nil {
			return m, waitForNotificationWSMessage(m.state.notification.wsMsgChan)
		}
		return m, nil

	case notificationWSDisconnectedMsg:
		log.Printf("Notification WebSocket disconnected")
		if msg.msgChan == m.state.notification.wsMsgChan {
			// Reset so the next page switch reconnects.
			m = m.stopNotificationWebSocket()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.theme = theme.BasicTheme(msg.IsDark(), nil)
		return m, nil
	case visibleError:
		m.error = &msg
	case toastExpiredMsg:
		if msg.id == m.state.toast.id {
			m.state.toast = toastState{}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.viewportWidth = msg.Width
		m.viewportHeight = msg.Height

		switch {
		case m.viewportWidth < 30 || m.viewportHeight < 12:
			m.size = undersized
			m.widthContainer = m.viewportWidth
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 60:
			m.size = small
			m.widthContainer = m.viewportWidth - 2
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 90:
			m.size = medium
			m.widthContainer = m.viewportWidth - 4
			m.heightContainer = min(msg.Height, 32)
		default:
			m.size = large
			m.widthContainer = 84
			m.heightContainer = min(msg.Height, 32)
		}

		m.widthContent = m.widthContainer
		m.heightContent = m.heightContainer
	case tea.KeyPressMsg:
		if key.Matches(msg, keys.Quit) {
			return m, tea.Sequence(m.cleanupCmd, tea.Quit)
		}

		// Overlays take every key while they are open.
		if m.state.menu.help {
			m.state.menu.help = false
			return m, nil
		}
		if m.state.notify.open && m.state.notify.confirmAction == RoomInviteAction {
			switch m.modalAnswer(msg) {
			case answerYes:
				return m, func() tea.Msg { return roomInviteAcceptedMsg{} }
			case answerNo:
				return m, func() tea.Msg { return roomInviteDeclinedMsg{} }
			case answerToggle:
				m.state.notify.focus = 1 - m.state.notify.focus
			}
			return m, nil
		}
		if m.state.menu.open {
			return m.MenuUpdate(msg)
		}

		if m.error != nil && key.Matches(msg, keys.Back) {
			if m.page == splashPage {
				return m, tea.Sequence(m.cleanupCmd, tea.Quit)
			}
			m.error = nil
			return m, nil
		}

		if m.wantsHelp(msg) {
			m.state.menu.help = true
			return m, nil
		}

		switch m.page {
		case newRoomPage, faqPage, settingsPage:
			switch msg.String() {
			case "m":
				return m.MenuSwitch()
			case "q":
				return m, tea.Sequence(m.cleanupCmd, tea.Quit)
			}
		}
	case CursorTickMsg:
		m, cmd := m.CursorUpdate(msg)
		return m, cmd
	case cleanupCompleteMsg:
		log.Println("Cleanup completed")
		return m, nil
	}

	var cmd tea.Cmd
	switch m.page {
	case splashPage:
		m, cmd = m.SplashUpdate(msg)
	case joinRoomPage:
		m, cmd = m.JoinRoomUpdate(msg)
	case newRoomPage:
		m, cmd = m.NewRoomUpdate(msg)
	case chatPage:
		m, cmd = m.ChatUpdate(msg)
	case settingsPage:
		m, cmd = m.SettingsUpdate(msg)
	case faqPage:
		m, cmd = m.FaqUpdate(msg)
	}

	// A key that just switched pages must not also act on the new page.
	if !m.switched {
		var headerCmd tea.Cmd
		m, headerCmd = m.HeaderUpdate(msg)
		cmds = append(cmds, headerCmd)
	}

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if m.switched {
		m.switched = false
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	if m.size == undersized {
		return m.ResizeView()
	}

	var view string
	switch m.page {
	case splashPage:
		return m.SplashView()
	case chatPage:
		// ChatView draws its own modals.
		// Over the message column, just above the input box.
		return m.withToastAt(m.ChatView(), m.chatLeftWidth()+m.chatCenterWidth()-1, chatInputHeight+chatStatusHeight)
	default:
		header := m.HeaderView()
		footer := m.FooterView()

		height := m.heightContainer - lipgloss.Height(header) - lipgloss.Height(footer)
		body := m.theme.Base().
			Width(m.widthContainer).
			Height(max(height, 1)).
			MaxHeight(max(height, 1)).
			Render(m.getContent(max(height, 1)))

		view = lipgloss.Place(
			m.viewportWidth,
			m.viewportHeight,
			lipgloss.Center,
			lipgloss.Center,
			lipgloss.JoinVertical(lipgloss.Left, header, body, footer),
		)
	}

	// Bottom-right of the centred layout, just above the status bar.
	right := (m.viewportWidth + m.widthContainer) / 2
	bottom := (m.viewportHeight-m.heightContainer)/2 + 3
	view = m.withToastAt(view, right, bottom)

	switch {
	case m.state.menu.help:
		view = m.overlay(m.HelpView(), view)
	case m.state.menu.open:
		view = m.overlay(m.MenuView(), view)
	case m.state.notify.open:
		view = m.overlay(m.RenderWarnModal(), view)
	}

	return view
}

// wantsHelp reports whether a key press should open the help overlay: "?"
// wherever it can't be text the user is typing.
func (m model) wantsHelp(msg tea.KeyPressMsg) bool {
	if msg.String() != "?" || m.state.notify.open {
		return false
	}
	switch m.page {
	case newRoomPage, faqPage, settingsPage:
		return true
	case joinRoomPage:
		return m.state.joinRoom.input.Value() == ""
	case chatPage:
		c := m.state.chat
		return !c.editMode && c.focusedInput == focusMessage && c.messageInput.Value() == ""
	}
	return false
}

func (m model) SwitchPage(page page) model {
	m.page = page
	m.switched = true
	return m
}

func (m model) getContent(height int) string {
	switch m.page {
	case newRoomPage:
		return m.NewRoomView(height)
	case joinRoomPage:
		return m.JoinRoomView(height)
	case faqPage:
		return m.FaqView(height)
	case settingsPage:
		return m.SettingsView(height)
	}
	return ""
}

func (m model) Cleanup() {
	if m.page == chatPage {
		m.leaveRoomRequest()()
	}

	if m.state.chat.wsCancel != nil {
		m.state.chat.wsCancel()
	}
	if m.state.chat.wsConn != nil {
		m.state.chat.wsConn.Close()
	}
	m.stopNotificationWebSocket()
}

// leaveRoomRequest returns a function that tells the server we left the
// current room (the owner deletes it, as the server forbids the owner from
// leaving). Closing the WebSocket alone keeps us listed as a member. All model
// state is captured up front so the function can run off the Update loop.
func (m model) leaveRoomRequest() func() {
	if m.client == nil || m.state.chat.room == nil || m.state.chat.room.ID == "" {
		return func() {}
	}
	client, roomID, isOwner, opts := m.client, m.state.chat.room.ID, m.state.chat.isRoomOwner, m.requestOpts()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var err error
		if isOwner {
			err = client.Room.Delete(ctx, roomID, opts...)
		} else {
			_, err = client.Room.Leave(ctx, roomID, opts...)
		}
		if err != nil {
			log.Printf("Error leaving room: %v", err)
		} else {
			log.Printf("Successfully notified server of room exit")
		}
	}
}

func (m model) cleanupCmd() tea.Msg {
	m.Cleanup()
	return cleanupCompleteMsg{}
}

func (m model) joinRoomFromInvite(invite *roomInviteData) tea.Cmd {
	return func() tea.Msg {
		opts := []option.RequestOption{}
		if m.userID != nil && *m.userID != "" {
			opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
		}

		room, err := m.client.Room.JoinByCodeWithToken(
			m.context,
			apisdk.JoinByCodeWithTokenParams{
				JoinCode:    invite.joinCode,
				SecureToken: invite.secureCode,
			},
			opts...,
		)
		if err != nil {
			log.Printf("Failed to join room from invite: %v", err)
			return visibleError{message: fmt.Sprintf("Failed to join room: %v", err)}
		}

		return roomJoinedMsg{room: room}
	}
}
