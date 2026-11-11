package tui

import (
	"context"
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

// A kick/expiry/deletion timer set in one room used to fire after the user had
// already left and joined another, throwing them out of the new room.
func TestStaleLeaveTimerDoesNotEjectFromNewRoom(t *testing.T) {
	m := newChatModel(t)
	old := m.state.chat.wsCtx
	m.clearChatState()
	m.page = newRoomPage
	m, _ = m.ChatSwitch(&apisdk.RoomResponse{ID: "r2", JoinCode: "DEF", ExpiresAt: time.Now().Add(time.Hour)})

	for _, msg := range []tea.Msg{wsKickTimeoutMsg{ctx: old}, wsRoomDeletedTimeoutMsg{ctx: old}, roomExpirationRedirectMsg{ctx: old}} {
		tm, _ := m.Update(msg)
		if mm := tm.(model); mm.page != chatPage || mm.state.chat.roomCode != "DEF" {
			t.Fatalf("%T from the previous room left the new one", msg)
		}
	}
	tm, _ := m.Update(wsKickTimeoutMsg{ctx: m.state.chat.wsCtx})
	if tm.(model).page == chatPage {
		t.Fatal("kick timeout of the current room was ignored")
	}
}

// An upload started in a previous room used to be auto-sent into the next one.
func TestUploadFromPreviousRoomIsIgnored(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.uploading = true
	m, cmd := m.ChatUpdate(fileUploadResultMsg{fileURL: "https://x/a.png", session: context.Background()})
	if cmd != nil || !m.state.chat.uploading {
		t.Fatal("upload result from another session was applied")
	}
}

// Rooms without an expiry (zero expires_at) used to "expire" after a second.
func TestRoomWithoutExpiryHasNoCountdown(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.expiresAt = time.Time{}
	if m.startExpirationCountdown() != nil {
		t.Fatal("countdown started for a room that never expires")
	}
}

func TestMemberSearchSurvivesMembershipChanges(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.participants = []apisdk.UserResponse{
		{ID: "1", Username: "alice"}, {ID: "2", Username: "carol"}, {ID: "3", Username: "bob"}, {ID: "4", Username: "dave"},
	}
	m.state.chat.searchActive = true
	m.state.chat.searchInput.SetValue("bob")

	// Search ran on the 4-member snapshot; carol leaves before it returns.
	res := m.searchParticipants("bob")()
	m, _ = m.ChatUpdate(wsMemberLeftMsg{userID: "2", username: "carol"})
	m, _ = m.ChatUpdate(res)
	if f := m.state.chat.filteredIndices; len(f) != 1 || m.state.chat.participants[f[0]].ID != "3" {
		t.Fatalf("filter = %v, want only bob", f)
	}

	// Another member leaving keeps the filter on bob.
	m, _ = m.ChatUpdate(wsMemberLeftMsg{userID: "1", username: "alice"})
	if f := m.state.chat.filteredIndices; len(f) != 1 || m.state.chat.participants[f[0]].ID != "3" {
		t.Fatalf("filter after leave = %v, want only bob", f)
	}

	// A result for an outdated query is dropped.
	m, _ = m.ChatUpdate(participantSearchResultMsg{query: "da", ids: []string{"4"}})
	if f := m.state.chat.filteredIndices; len(f) != 1 || m.state.chat.participants[f[0]].ID != "3" {
		t.Fatalf("stale result applied: %v", f)
	}
}

// A second create/join finishing while already in a room used to swap only
// the room pointer, sending to one room while listening to another.
func TestSecondRoomJoinWhileInChatIsIgnored(t *testing.T) {
	m := newChatModel(t)
	m.page = chatPage
	tm, _ := m.Update(roomJoinedMsg{room: &apisdk.RoomResponse{ID: "other", JoinCode: "ZZZ"}})
	if got := tm.(model).state.chat.room.ID; got != "r" {
		t.Fatalf("room switched to %q", got)
	}
}

// The invite QR code kept encoding the old join code after it was changed.
func TestQRCodeFollowsNewJoinCode(t *testing.T) {
	m := newChatModel(t)
	room := *m.state.chat.room
	room.QRCodeURL = "https://visper.example/?joinCode=ABC&secureCode=s3"
	m.state.chat.room = &room
	m, _ = m.ChatUpdate(wsRoomUpdatedMsg{joinCode: "NEW"})
	if got := m.state.chat.room.QRCodeURL; !strings.Contains(got, "joinCode=NEW") || !strings.Contains(got, "secureCode=s3") {
		t.Fatalf("QR URL = %q", got)
	}
}
