// Package tui is the Bubble Tea terminal UI of the Visper client.
//
// The UI is one tea.Model (model) whose state is split per page and per
// overlay (see state). Files are grouped by concern:
//
//	model.go, update.go, view.go   the root model and its message routing
//	page_*.go                      the top-level pages (splash, new/join room, FAQ, settings)
//	chat*.go                       the chat room page
//	room_ws.go, notification_ws.go the WebSocket listeners
//	header.go, footer.go, menu.go, help.go, dialog.go, toast.go, widgets.go
//	                               shared chrome and building blocks
package tui

import (
	"context"
	"log"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/hilthontt/visper/clients/cli/internal/preview"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// page is a full-screen view of the app.
type page int

const (
	splashPage page = iota
	faqPage
	newRoomPage
	joinRoomPage
	chatPage
	settingsPage
)

// layoutSize is the breakpoint the window currently falls in.
type layoutSize int

const (
	undersized layoutSize = iota
	small
	medium
	large
)

// state holds the per-page and per-overlay state of the model.
type state struct {
	anim         animState
	footer       footerState
	menu         menuState
	dialog       dialogState
	toast        toastState
	notification notificationState

	splash   splashState
	newRoom  newRoomState
	joinRoom joinRoomState
	faq      faqState
	settings settingsState
	chat     chatState
}

// visibleError is an error shown to the user: on the splash screen, or as a
// banner above the footer of the other pages.
type visibleError struct {
	message string
}

// roomJoinedMsg reports a room that was created or joined; it opens the chat.
type roomJoinedMsg struct {
	room *apisdk.RoomResponse
}

type model struct {
	ctx    context.Context
	client *apisdk.Client
	// userID identifies this client to the API (the X-User-ID header). The
	// server may assign a different one when a room is joined.
	userID string

	page page
	// switched is set by a key that changed the page during this Update, so
	// the same key isn't also handled by the new page's header shortcuts.
	switched bool
	state    state
	error    *visibleError

	viewportWidth   int
	viewportHeight  int
	widthContainer  int
	heightContainer int
	size            layoutSize

	theme           theme.Theme
	faqs            []faq
	companions      []companion
	imagePreviewer  *preview.ImagePreviewer
	settingsManager settings.Manager
}

// NewModel returns the root model of the TUI.
func NewModel() (tea.Model, error) {
	userID := uuid.NewString()
	log.Printf("USERID: %s\n", userID)

	return model{
		ctx:             context.Background(),
		userID:          userID,
		page:            splashPage,
		theme:           theme.BasicTheme(true, nil),
		faqs:            loadFaqs(),
		companions:      loadCompanions(),
		imagePreviewer:  preview.NewImagePreviewer(),
		settingsManager: settings.NewManager(),
	}, nil
}

func (m model) Init() tea.Cmd {
	return m.splashInit()
}

// isCurrentUser reports whether id is the user running this client.
func (m model) isCurrentUser(id string) bool {
	return m.userID != "" && id == m.userID
}

// switchPage changes the current page and marks the update as switched.
func (m model) switchPage(p page) model {
	m.page = p
	m.switched = true
	return m
}

// gotoPage opens one of the top-level (tabbed) pages.
func (m model) gotoPage(p page) (model, tea.Cmd) {
	switch p {
	case newRoomPage:
		return m.gotoNewRoom()
	case joinRoomPage:
		return m.gotoJoinRoom()
	case faqPage:
		return m.gotoFaq()
	case settingsPage:
		return m.gotoSettings()
	default:
		return m, nil
	}
}
