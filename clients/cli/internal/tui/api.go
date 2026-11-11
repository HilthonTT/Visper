package tui

import (
	"context"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/resource"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

// newAPIClient returns an SDK client for the configured API.
func newAPIClient() *apisdk.Client {
	return apisdk.NewClient(option.WithBaseURL(resource.Resource.Api.Url))
}

// requestOpts returns the per-request options identifying the current user.
func (m model) requestOpts() []option.RequestOption {
	if m.userID == "" {
		return nil
	}
	return []option.RequestOption{option.WithHeader("X-User-ID", m.userID)}
}

// requestFailedMsg reports a failed background API call; action completes
// "Couldn't …" in the toast shown for it.
type requestFailedMsg struct {
	action string
	err    error
}

// apiRequest runs fn as a tea.Cmd (off the Update loop) and reports a failure
// as a requestFailedMsg. Everything fn needs must be captured by the caller up
// front; it must not touch the model.
func (m model) apiRequest(action string, fn func(ctx context.Context, opts []option.RequestOption) error) tea.Cmd {
	ctx, opts := m.ctx, m.requestOpts()
	return func() tea.Msg {
		if err := fn(ctx, opts); err != nil {
			log.Printf("Failed to %s: %v", action, err)
			return requestFailedMsg{action: action, err: err}
		}
		return nil
	}
}

// leaveRoomFunc returns a function that leaves (or, for its owner, deletes)
// roomID; the server forbids the owner from leaving. Closing the WebSocket
// alone keeps us listed as a member. It touches no model state, so it can run
// off the Update loop.
func leaveRoomFunc(client *apisdk.Client, roomID string, isOwner bool, opts []option.RequestOption) func() {
	if client == nil || roomID == "" {
		return func() {}
	}
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

// leaveCurrentRoomFunc is leaveRoomFunc for the room the user is in, if any.
func (m model) leaveCurrentRoomFunc() func() {
	room := m.state.chat.room
	if room == nil {
		return func() {}
	}
	return leaveRoomFunc(m.client, room.ID, m.state.chat.isRoomOwner, m.requestOpts())
}

// cleanup leaves the current room and closes every connection.
func (m model) cleanup() {
	if m.page == chatPage {
		m.leaveCurrentRoomFunc()()
	}
	m.state.chat.closeConnections()
	m.stopNotificationListener()
}

// quit cleans up, then exits the program.
func (m model) quit() tea.Cmd {
	return tea.Sequence(func() tea.Msg {
		m.cleanup()
		return cleanupCompleteMsg{}
	}, tea.Quit)
}
