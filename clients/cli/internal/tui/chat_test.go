package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func newChatModel(t *testing.T) model {
	t.Helper()
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()
	m, _ = m.ChatSwitch(&apisdk.RoomResponse{ID: "r", JoinCode: "ABC", ExpiresAt: time.Now().Add(time.Hour)})
	t.Cleanup(m.clearChatState)
	return m
}

// Widths between the "small" breakpoint and the width the three-column chat
// layout needs used to give the message column a negative width and panic.
func TestChatViewDoesNotPanicAtAnySize(t *testing.T) {
	m := newChatModel(t)
	for _, sz := range [][2]int{{200, 50}, {80, 30}, {72, 20}, {70, 20}, {60, 15}, {50, 10}, {30, 5}, {0, 0}} {
		tm, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		mm := tm.(model)
		_ = mm.render()
		_ = mm.openWarnModalForLeaveRoom().render()
	}
}

func TestStaleRoomWebSocketMessagesAreDropped(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.wsMsgChan = make(chan tea.Msg, 1)
	stale := make(chan tea.Msg)

	tm, _ := m.Update(wsChanMsg{ch: stale, msg: wsDisconnectedMsg{}})
	if tm.(model).state.notify.open {
		t.Fatal("disconnect of a previous room's channel opened a modal")
	}
	tm, _ = m.Update(wsChanMsg{ch: stale, msg: wsMessageReceivedMsg{message: apisdk.MessageResponse{ID: "x"}}})
	if n := len(tm.(model).state.chat.messages); n != 0 {
		t.Fatalf("message from a previous room was appended (%d messages)", n)
	}
	tm, _ = m.Update(wsChanMsg{ch: m.state.chat.wsMsgChan, msg: wsMessageReceivedMsg{message: apisdk.MessageResponse{ID: "y"}}})
	if n := len(tm.(model).state.chat.messages); n != 1 {
		t.Fatalf("current room message not applied (%d messages)", n)
	}
}

func TestEditSelectionSurvivesDeletions(t *testing.T) {
	m := newChatModel(t)
	me := *m.userID
	for _, id := range []string{"a", "b", "c", "d"} {
		m.state.chat.messages = append(m.state.chat.messages, apisdk.MessageResponse{ID: id, UserID: me})
	}
	m.state.chat.editMode = true
	m.state.chat.selectedMessageIndex = 3 // "d"

	m, _ = m.ChatUpdate(wsMessageDeletedMsg{messageID: "a"})
	if got := m.state.chat.messages[m.state.chat.selectedMessageIndex].ID; got != "d" {
		t.Fatalf("selection moved to %q, want d", got)
	}

	// An out-of-range selection must not panic when moving up.
	m.state.chat.selectedMessageIndex = 10
	if got := m.getPreviousOwnMessageIndex(10); got != 2 {
		t.Fatalf("getPreviousOwnMessageIndex(10) = %d, want 2", got)
	}
}

// Long messages used to render on one line and get cut off by the viewport.
func TestLongMessagesWrapToViewportWidth(t *testing.T) {
	m := newChatModel(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = tm.(model)
	long := strings.Repeat("end-to-end encrypted words ", 12)
	m.state.chat.messages = []apisdk.MessageResponse{{ID: "1", Username: "friend", Content: long}}

	width := m.state.chat.messagesViewport.Width()
	out := m.renderMessages()
	for line := range strings.SplitSeq(out, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line is %d cells wide, viewport is %d: %q", w, width, line)
		}
	}
	if !strings.Contains(strings.ReplaceAll(out, "\n", " "), "encrypted") || strings.Count(out, "\n") < 4 {
		t.Fatalf("expected the message to wrap over several lines:\n%s", out)
	}
}

// The viewport used to keep its 50x20 defaults until the next resize, because
// View sizes a copy of the model.
func TestChatViewportIsSizedWhenRoomOpens(t *testing.T) {
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 131, Height: 33})
	m = tm.(model)

	m, _ = m.ChatSwitch(&apisdk.RoomResponse{ID: "r", JoinCode: "ABC", ExpiresAt: time.Now().Add(time.Hour)})
	t.Cleanup(m.clearChatState)

	vp := m.state.chat.messagesViewport
	if vp.Width() != m.chatCenterWidth() || vp.Height() != m.chatColumnHeight()-4 {
		t.Fatalf("viewport is %dx%d, want %dx%d", vp.Width(), vp.Height(), m.chatCenterWidth(), m.chatColumnHeight()-4)
	}
}
