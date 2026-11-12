package tui

import (
	"context"
	"errors"
	"fmt"
	"log"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The notification listener receives room invites for this user. It runs
// while the user is outside a room; entering a room stops it.

type notificationState struct {
	conn   *apisdk.NotificationWebSocket
	ctx    context.Context
	cancel context.CancelFunc
	events chan tea.Msg
	// pendingInvite is the invite shown in the invite dialog.
	pendingInvite *roomInviteMsg
}

type (
	notificationConnectedMsg struct {
		conn *apisdk.NotificationWebSocket
		ctx  context.Context
	}
	notificationConnectFailedMsg struct {
		ctx context.Context
	}
	notificationListeningMsg struct {
		events chan tea.Msg
		ctx    context.Context
	}
	notificationErrorMsg struct {
		code    string
		message string
	}
	notificationDisconnectedMsg struct {
		events chan tea.Msg
	}
	roomInviteMsg struct {
		roomID     string
		joinCode   string
		secureCode string
		// sealedKey is the room key, encrypted to this terminal's device key.
		sealedKey string
	}
)

// startNotificationListener starts the listener unless one is already
// connected or connecting.
func (m model) startNotificationListener() (model, tea.Cmd) {
	if m.state.notification.cancel != nil || m.client == nil || m.userID == "" {
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.state.notification = notificationState{ctx: ctx, cancel: cancel}

	client, opts := m.client, m.requestOpts()
	return m, func() tea.Msg {
		ws, err := client.ConnectNotificationWebSocket(ctx, opts...)
		if err != nil {
			log.Printf("Failed to connect notification WebSocket: %v", err)
			return notificationConnectFailedMsg{ctx: ctx}
		}
		return notificationConnectedMsg{conn: ws, ctx: ctx}
	}
}

// stopNotificationListener cancels and closes the listener and resets its
// state.
func (m model) stopNotificationListener() model {
	n := m.state.notification
	if n.cancel != nil {
		n.cancel()
	}
	if n.conn != nil {
		n.conn.Close()
	}
	m.state.notification = notificationState{}
	return m
}

// notificationUpdate handles the listener's messages; handled is false for
// any other message.
func (m model) notificationUpdate(msg tea.Msg) (_ model, _ tea.Cmd, handled bool) {
	n := &m.state.notification

	switch msg := msg.(type) {
	case notificationConnectedMsg:
		if msg.ctx != n.ctx || msg.ctx.Err() != nil {
			// The listener was cancelled (e.g. we entered a room) while dialing.
			msg.conn.Close()
			return m, nil, true
		}
		n.conn = msg.conn
		return m, listenNotifications(msg.ctx, msg.conn), true

	case notificationConnectFailedMsg:
		if msg.ctx == n.ctx {
			m = m.stopNotificationListener()
		}
		return m, nil, true

	case notificationListeningMsg:
		if msg.ctx != n.ctx {
			// A listener we already stopped (e.g. we entered a room while it
			// was starting). Adopting its channel would orphan the current one.
			return m, nil, true
		}
		n.events = msg.events
		return m, waitForNotification(msg.events), true

	case roomInviteMsg:
		if m.page != chatPage {
			n.pendingInvite = &msg
			m = m.openRoomInviteDialog(msg.roomID)
		}
		return m, waitForNotification(m.state.notification.events), true

	case notificationErrorMsg:
		log.Printf("Notification WS error: %s - %s", msg.code, msg.message)
		return m, waitForNotification(n.events), true

	case notificationDisconnectedMsg:
		log.Printf("Notification WebSocket disconnected")
		if msg.events == n.events {
			// Reset so the next page switch reconnects.
			m = m.stopNotificationListener()
		}
		return m, nil, true
	}
	return m, nil, false
}

// roomInviteDialogUpdate handles keys while the invite dialog is open.
func (m model) roomInviteDialogUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	switch m.confirmAnswer(msg) {
	case answerYes:
		invite := m.state.notification.pendingInvite
		m.state.notification.pendingInvite = nil
		m = m.closeDialog()
		if invite == nil {
			return m, nil
		}
		return m, m.joinFromInvite(*invite)
	case answerNo:
		m.state.notification.pendingInvite = nil
		return m.closeDialog(), nil
	case answerToggle:
		m.state.dialog.focus = 1 - m.state.dialog.focus
	}
	return m, nil
}

func (m model) joinFromInvite(invite roomInviteMsg) tea.Cmd {
	client, ctx, opts, deviceKey := m.client, m.ctx, m.requestOpts(), m.deviceKey
	return func() tea.Msg {
		key, err := deviceKey.OpenRoomKey(invite.sealedKey)
		if err != nil {
			log.Printf("Failed to open the invite's room key: %v", err)
			return visibleError{message: "Couldn't unlock this invite. Send it again with the device code from this terminal's settings page (it changes every time Visper starts)."}
		}
		room, err := client.Room.JoinByCodeWithToken(ctx, apisdk.JoinByCodeWithTokenParams{
			JoinCode:    invite.joinCode,
			SecureToken: invite.secureCode,
		}, opts...)
		if err != nil {
			log.Printf("Failed to join room from invite: %v", err)
			return visibleError{message: fmt.Sprintf("Failed to join room: %v", err)}
		}
		return roomJoinedMsg{room: room, roomKey: key}
	}
}

// listenNotifications starts reading conn and returns the channel its events
// are delivered on, as a notificationListeningMsg.
func listenNotifications(ctx context.Context, conn *apisdk.NotificationWebSocket) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 100)
		send := func(msg tea.Msg) {
			select {
			case events <- msg:
			case <-ctx.Done():
			}
		}

		conn.SetMessageHandler(func(wsMsg apisdk.NotificationWSMessage) {
			if ctx.Err() != nil {
				return
			}
			if msg, ok := parseNotification(wsMsg); ok {
				send(msg)
			}
		})

		go func() {
			defer close(events)
			if err := conn.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Notification WebSocket listen error: %v", err)
				send(notificationDisconnectedMsg{events: events})
			}
		}()

		return notificationListeningMsg{events: events, ctx: ctx}
	}
}

// parseNotification turns a notification into a tea.Msg; ok is false for
// unknown or invalid ones.
func parseNotification(wsMsg apisdk.NotificationWSMessage) (tea.Msg, bool) {
	data := wsMsg.Data
	switch wsMsg.Type {
	case apisdk.NotificationRoomInvite:
		roomID, _ := data["room_id"].(string)
		joinCode, _ := data["join_code"].(string)
		secureCode, _ := data["secure_code"].(string)
		sealedKey, _ := data["sealed_key"].(string)
		if roomID == "" || joinCode == "" || secureCode == "" || sealedKey == "" {
			log.Printf("Invalid room invite payload for room %q", roomID)
			return nil, false
		}
		return roomInviteMsg{roomID: roomID, joinCode: joinCode, secureCode: secureCode, sealedKey: sealedKey}, true

	case apisdk.NotificationError:
		code, _ := data["code"].(string)
		message, _ := data["message"].(string)
		return notificationErrorMsg{code: code, message: message}, true
	}
	return nil, false
}

// waitForNotification reads the next event; a closed channel reads as a
// disconnect.
func waitForNotification(events chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return notificationDisconnectedMsg{events: events}
		}
		return msg
	}
}
