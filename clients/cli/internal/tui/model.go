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
// roomKey is the room's encryption key; the server never has it.
type roomJoinedMsg struct {
	room    *apisdk.RoomResponse
	roomKey string
}

type model struct {
	ctx    context.Context
	client *apisdk.Client
	// userToken is this client's private credential (the X-User-Token
	// header). It is never shown or logged.
	userToken string
	// userID is the public ID the API derives from userToken; it is what
	// messages and member lists show.
	userID string
	// deviceKey lets the web client send this terminal a room key through the
	// API without the API being able to read it (see the device code on the
	// settings page).
	deviceKey *apisdk.DeviceKey
	// identity holds the signing key that signs everything this client
	// sends, with the device key above. Other members learn it from the key
	// card we publish in each room (see apisdk.RoomCrypto).
	identity *apisdk.Identity
	// identityStore keeps the user token, the identity and the rooms we are
	// in across runs (see identity.go); nil if they last only this run.
	identityStore *settings.IdentityStore
	// aiConsent is set once the user agreed to send drafts to the AI
	// service, which reads them unencrypted.
	aiConsent bool

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
	store, userToken, identity, err := loadIdentity(identityPath())
	if err != nil {
		return nil, err
	}
	userID := identity.UserID()
	log.Printf("USERID: %s (kept across runs: %v)\n", userID, store != nil)

	return model{
		ctx:             context.Background(),
		userToken:       userToken,
		userID:          userID,
		deviceKey:       identity.Device(),
		identity:        identity,
		identityStore:   store,
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
	var cmd tea.Cmd
	switch p {
	case newRoomPage:
		m, cmd = m.gotoNewRoom()
	case joinRoomPage:
		m, cmd = m.gotoJoinRoom()
	case faqPage:
		m, cmd = m.gotoFaq()
	case settingsPage:
		m, cmd = m.gotoSettings()
	default:
		return m, nil
	}
	// Reconnect a dropped notification listener on any page, not only New
	// room, so invites keep arriving (a no-op while it is connected).
	m, notifyCmd := m.startNotificationListener()
	return m, tea.Batch(cmd, notifyCmd)
}
