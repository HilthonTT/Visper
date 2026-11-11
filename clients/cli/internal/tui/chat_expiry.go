package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

const (
	// expiryWarning is how long before expiry the user is warned.
	expiryWarning = 5 * time.Minute
	// roomClosedDelay is how long the "room expired" dialog stays up before
	// returning to the start page.
	roomClosedDelay = 3 * time.Second
)

// expiryState is the countdown to the room's expiry shown in the top bar.
type expiryState struct {
	expiresAt time.Time
	remaining string
	warned    bool
	// ctx stops the countdown; ticks carry it so ones from a previous room
	// are ignored.
	ctx    context.Context
	cancel context.CancelFunc
}

type (
	expiryTickMsg struct {
		remaining time.Duration
		ctx       context.Context
	}
	roomExpiredMsg struct {
		ctx context.Context
	}
)

func newExpiryState(session context.Context, expiresAt time.Time) expiryState {
	ctx, cancel := context.WithCancel(session)
	return expiryState{expiresAt: expiresAt, ctx: ctx, cancel: cancel}
}

// startExpirationCountdown schedules the next countdown tick.
func (m model) startExpirationCountdown() tea.Cmd {
	ctx, expiresAt := m.state.chat.expiry.ctx, m.state.chat.expiry.expiresAt
	if expiresAt.IsZero() {
		// The API sends a zero expires_at for rooms that never expire;
		// counting down to it would "expire" the room after one second.
		return nil
	}
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
		remaining := time.Until(expiresAt)
		if remaining <= 0 {
			return roomExpiredMsg{ctx: ctx}
		}
		return expiryTickMsg{remaining: remaining, ctx: ctx}
	}
}

func (m model) expiryTickUpdate(msg expiryTickMsg) (model, tea.Cmd) {
	e := &m.state.chat.expiry
	if msg.ctx != e.ctx {
		return m, nil // countdown of a room we already left
	}
	e.remaining = formatDuration(msg.remaining)

	if msg.remaining <= expiryWarning && !e.warned {
		e.warned = true
		m.state.chat.companion.mood = "hurry, time's almost up!"
		m, cmd := m.toast(theme.ToneWarning, "This room expires in 5 minutes. Messages will be lost.")
		return m, tea.Batch(cmd, m.startExpirationCountdown())
	}
	return m, m.startExpirationCountdown()
}

func (m model) roomExpiredUpdate(msg roomExpiredMsg) (model, tea.Cmd) {
	if msg.ctx != m.state.chat.expiry.ctx {
		return m, nil
	}
	m.state.chat.expiry.cancel()
	return m.openRoomExpiredDialog(), m.closeRoomAfter(roomClosedDelay)
}

// formatDuration formats a countdown as mm:ss, or hh:mm:ss from an hour up.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
