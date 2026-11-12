package tui

import (
	"log"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// cleanupCompleteMsg reports that cleanup (leaving the room, closing the
// WebSockets) finished before quitting.
type cleanupCompleteMsg struct{}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.switched = false

	if env, ok := msg.(roomEventMsg); ok {
		if m.page != chatPage || env.ch != m.state.chat.events {
			return m, nil // stale event from a room we already left
		}
		msg = env.msg
	}

	if mm, cmd, handled := m.notificationUpdate(msg); handled {
		return mm, cmd
	}

	switch msg := msg.(type) {
	case roomJoinedMsg:
		return m.enterChat(msg.room, msg.roomKey)
	case tea.BackgroundColorMsg:
		m.theme = theme.BasicTheme(msg.IsDark(), nil)
		return m, nil
	case toastExpiredMsg:
		if msg.id == m.state.toast.id {
			m.state.toast = toastState{}
		}
		return m, nil
	case animTickMsg:
		return m.animUpdate()
	case cleanupCompleteMsg:
		log.Println("Cleanup completed")
		return m, nil
	case visibleError:
		m.error = &msg
	case tea.WindowSizeMsg:
		m = m.resize(msg)
	case tea.KeyPressMsg:
		if mm, cmd, handled := m.globalKeyUpdate(msg); handled {
			return mm, cmd
		}
	}

	m, cmd := m.pageUpdate(msg)
	cmds := []tea.Cmd{cmd}

	// A key that just switched pages must not also act on the new page.
	if !m.switched {
		var headerCmd tea.Cmd
		m, headerCmd = m.headerUpdate(msg)
		cmds = append(cmds, headerCmd)
	}
	m.switched = false

	return m, tea.Batch(cmds...)
}

// pageUpdate hands msg to the current page.
func (m model) pageUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch m.page {
	case splashPage:
		return m.splashUpdate(msg)
	case newRoomPage:
		return m.newRoomUpdate(msg)
	case joinRoomPage:
		return m.joinRoomUpdate(msg)
	case faqPage:
		return m.faqUpdate(msg)
	case settingsPage:
		return m.settingsUpdate(msg)
	case chatPage:
		return m.chatUpdate(msg)
	}
	return m, nil
}

// globalKeyUpdate handles the keys that work on every page, and the overlays
// that take every key while they are open. handled is false when the key
// should go on to the page.
func (m model) globalKeyUpdate(msg tea.KeyPressMsg) (_ model, _ tea.Cmd, handled bool) {
	if key.Matches(msg, keys.Quit) {
		return m, m.quit(), true
	}

	switch {
	case m.state.menu.help:
		m.state.menu.help = false
		return m, nil, true
	case m.state.dialog.open && m.state.dialog.kind == dialogRoomInvite:
		mm, cmd := m.roomInviteDialogUpdate(msg)
		return mm, cmd, true
	case m.state.menu.open:
		mm, cmd := m.menuUpdate(msg)
		return mm, cmd, true
	}

	if m.error != nil && key.Matches(msg, keys.Back) {
		if m.page == splashPage {
			return m, m.quit(), true
		}
		m.error = nil
		return m, nil, true
	}

	if m.wantsHelp(msg) {
		m.state.menu.help = true
		return m, nil, true
	}

	switch m.page {
	case newRoomPage, faqPage, settingsPage:
		switch {
		case key.Matches(msg, keys.Menu):
			mm, cmd := m.openMenu()
			return mm, cmd, true
		case key.Matches(msg, keys.QuitPage):
			return m, m.quit(), true
		}
	}
	return m, nil, false
}

// wantsHelp reports whether a key press should open the help overlay: "?"
// wherever it can't be text the user is typing.
func (m model) wantsHelp(msg tea.KeyPressMsg) bool {
	if !key.Matches(msg, keys.Help) || m.state.dialog.open {
		return false
	}
	switch m.page {
	case newRoomPage, faqPage, settingsPage:
		return true
	case joinRoomPage:
		return m.state.joinRoom.input.Value() == ""
	case chatPage:
		c := m.state.chat
		return !c.edit.active && !c.members.searching && c.messageInput.Value() == ""
	}
	return false
}

// resize recomputes the layout breakpoint and container size for a new
// window size.
func (m model) resize(msg tea.WindowSizeMsg) model {
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
	return m
}
