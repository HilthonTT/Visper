package tui

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
	filepreview "github.com/hilthontt/visper/cli/pkg/file_preview"
	"github.com/hilthontt/visper/cli/pkg/generator"
	"github.com/hilthontt/visper/cli/pkg/settings_manager"
	stringfunction "github.com/hilthontt/visper/cli/pkg/string_function"
	"github.com/hilthontt/visper/cli/pkg/tui/theme"
)

type page = int
type size = int

const (
	splashPage page = iota
	faqPage
	newRoomPage
	joinRoomPage
	chatPage
	menuPage
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
	notify       notifyState
	notification notificationListenerState
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
	renderer        *lipgloss.Renderer
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
	generator       *generator.Generator
	imagePreviewer  *filepreview.ImagePreviewer
	settingsManager settings_manager.SettingsManager
	username        *string
	userID          *string
}

func NewModel(renderer *lipgloss.Renderer, generator *generator.Generator) (tea.Model, error) {
	ctx := context.Background()

	userID := uuid.NewString()

	// TODO Display in header
	log.Printf("USERID: %s\n", userID)

	m := model{
		context:  ctx,
		page:     splashPage,
		renderer: renderer,
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
		theme:           theme.BasicTheme(renderer, nil),
		faqs:            LoadFaqs(),
		waifus:          LoadWaifus(),
		generator:       generator,
		imagePreviewer:  filepreview.NewImagePreviewer(),
		settingsManager: settings_manager.NewSettingsManager(),
		userID:          &userID,
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.SplashInit(),
	}

	// Only initialize the context, don't connect yet
	if m.userID != nil && *m.userID != "" {
		ctx, cancel := context.WithCancel(context.Background())
		m.state.notification = notificationListenerState{
			wsCtx:    ctx,
			wsCancel: cancel,
		}
		// Don't connect here - client doesn't exist yet!
		// Connection will happen in SplashUpdate after client is ready
	}

	return tea.Batch(cmds...)
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}

	switch msg := msg.(type) {
	case roomJoinedMsg:
		return m.ChatSwitch(msg.room)
	case notificationWSConnectedMsg:
		m.state.notification.wsConn = msg.conn
		return m, m.listenNotificationWebSocket()

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
		return m, nil
	case visibleError:
		m.error = &msg
	case tea.WindowSizeMsg:
		m.viewportWidth = msg.Width
		m.viewportHeight = msg.Height

		switch {
		case m.viewportWidth < 20 || m.viewportHeight < 10:
			m.size = undersized
			m.widthContainer = m.viewportWidth
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 50:
			m.size = small
			m.widthContainer = m.viewportWidth
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 80:
			m.size = medium
			m.widthContainer = 50
			m.heightContainer = int(math.Min(float64(msg.Height), 30))
		default:
			m.size = large
			m.widthContainer = 80
			m.heightContainer = int(math.Min(float64(msg.Height), 30))
		}

		m.widthContent = m.widthContainer - 2
		m.heightContent = m.heightContainer
	case tea.KeyMsg:
		if m.state.notify.open && m.state.notify.confirmAction == RoomInviteAction {
			switch msg.String() {
			case "y", "Y", "enter":
				return m, func() tea.Msg {
					return roomInviteAcceptedMsg{}
				}
			case "n", "N", "esc":
				return m, func() tea.Msg {
					return roomInviteDeclinedMsg{}
				}
			}
		}

		switch {
		case key.Matches(msg, keys.Back):
			if m.error != nil {
				if m.page == splashPage {
					return m, tea.Sequence(
						m.cleanupCmd,
						tea.Quit,
					)
				}
				m.error = nil
				return m, nil
			}
		case key.Matches(msg, keys.Quit):
			return m, tea.Sequence(
				m.cleanupCmd,
				tea.Quit,
			)
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
	case menuPage:
		m, cmd = m.MenuUpdate(msg)
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
	}

	var headerCmd tea.Cmd
	m, headerCmd = m.HeaderUpdate(msg)
	cmds = append(cmds, headerCmd)

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if m.switched {
		m.switched = false
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.size == undersized {
		return m.ResizeView()
	}

	var baseView string

	switch m.page {
	case splashPage:
		baseView = m.SplashView()
	case chatPage:
		baseView = m.ChatView()
	case menuPage:
		baseView = m.MenuView()
	default:
		header := m.HeaderView()
		footer := m.FooterView()

		// Get content based on current page
		content := m.getContent()

		height := m.heightContainer
		height -= lipgloss.Height(header)
		height -= lipgloss.Height(footer)

		body := m.theme.Base().Width(m.widthContainer).Height(height).Render(content)

		sb := strings.Builder{}
		sb.WriteString(header)
		sb.WriteString(body)
		sb.WriteString(footer)

		child := lipgloss.JoinVertical(
			lipgloss.Left,
			sb.String(),
		)

		baseView = m.renderer.Place(
			m.viewportWidth,
			m.viewportHeight,
			lipgloss.Center,
			lipgloss.Center,
			m.theme.Base().
				MaxWidth(m.widthContainer).
				MaxHeight(m.heightContainer).
				Render(child),
		)
	}

	if m.state.notify.open && m.page != chatPage {
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

func (m model) SwitchPage(page page) model {
	m.page = page
	m.switched = true
	return m
}

func (m model) getContent() string {
	page := "unknown"
	switch m.page {
	case newRoomPage:
		page = m.NewRoomView()
	case joinRoomPage:
		page = m.JoinRoomView()
	case faqPage:
		page = m.FaqView()
	case settingsPage:
		page = m.SettingsView()
	}
	return page
}

func (m model) Cleanup() {
	if m.page == chatPage && m.client != nil {
		roomID := m.state.chat.room.ID
		if roomID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			opts := []option.RequestOption{}
			if m.userID != nil && *m.userID != "" {
				opts = append(opts, option.WithHeader("X-User-ID", *m.userID))
			}

			var err error

			if m.state.chat.isRoomOwner {
				err = m.client.Room.Delete(ctx, roomID, opts...)
			} else {
				_, err = m.client.Room.Leave(ctx, roomID, opts...)
			}

			if err != nil {
				log.Printf("Error leaving room during cleanup: %v", err)
			} else {
				log.Printf("Successfully notified server of room exit")
			}
		}
	}

	if m.state.notification.wsCancel != nil {
		m.state.notification.wsCancel()
	}
	if m.state.notification.wsConn != nil {
		m.state.notification.wsConn.Close()
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

		room, err := m.client.Room.GetByJoinCode(
			m.context,
			apisdk.JoinByCodeParams{
				JoinCode: invite.joinCode,
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
