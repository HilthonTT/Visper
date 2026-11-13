package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func withMessages(m model, msgs ...apisdk.MessageResponse) model {
	m.state.chat.messages = msgs
	return m.refreshMessages()
}

func TestParseSocialRoomEvents(t *testing.T) {
	identity := func(s string, _ bool, _ sealedAs) string { return s }
	tests := []struct {
		msg  apisdk.WSMessage
		want any
	}{
		{apisdk.WSMessage{Type: apisdk.PresenceUpdate, Data: map[string]any{"online": []any{"a", "b"}}}, presenceMsg{online: []string{"a", "b"}}},
		{apisdk.WSMessage{Type: apisdk.MemberTyping, Data: map[string]any{"userId": "a", "username": "Ann"}}, memberTypingMsg{userID: "a", username: "Ann"}},
		{apisdk.WSMessage{Type: apisdk.ReadUpdate, Data: map[string]any{"userId": "a", "messageId": "m1"}}, readUpdateMsg{userID: "a", messageID: "m1"}},
	}
	for _, tt := range tests {
		got, ok := parseRoomEvent(tt.msg, identity)
		if !ok {
			t.Fatalf("%s not parsed", tt.msg.Type)
		}
		if gs, ws := strings.TrimSpace(sprint(got)), strings.TrimSpace(sprint(tt.want)); gs != ws {
			t.Errorf("%s = %s, want %s", tt.msg.Type, gs, ws)
		}
	}

	decrypt := func(s string, enc bool, _ sealedAs) string {
		if enc {
			return "dec(" + s + ")"
		}
		return s
	}
	got, ok := parseRoomEvent(apisdk.WSMessage{Type: apisdk.MessageReceived, Data: map[string]any{
		"id": "m2", "userId": "a", "username": "Ann", "content": "c", "encrypted": true,
		"replyToId": "m1",
		"reactions": []any{map[string]any{"userId": "b", "emoji": "x", "encrypted": true}},
	}}, decrypt)
	if !ok {
		t.Fatal("message not parsed")
	}
	msg := got.(messageReceivedMsg).message
	if msg.ReplyToID != "m1" || len(msg.Reactions) != 1 || msg.Reactions[0].Emoji != "dec(x)" {
		t.Fatalf("message = %+v", msg)
	}
}

func sprint(v any) string { return fmt.Sprintf("%#v", v) }

func TestSelectModeRepliesAndReactsToOthersMessages(t *testing.T) {
	m := newChatModel(t)
	m = withMessages(m,
		apisdk.MessageResponse{ID: "m1", UserID: "friend", Username: "friend", Content: "hello there"},
		apisdk.MessageResponse{ID: "m2", UserID: m.userID, Username: "me", Content: "hi"},
	)
	m, _ = m.enterEditMode()
	m, _ = m.editModeUpdate(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.state.chat.edit.selected != 0 {
		t.Fatalf("selected = %d, want 0 (someone else's message)", m.state.chat.edit.selected)
	}

	// Editing someone else's message is refused.
	mm, _ := m.editModeUpdate(tea.KeyPressMsg{Code: tea.KeyEnter})
	if mm.state.dialog.open {
		t.Fatal("opened the edit dialog for someone else's message")
	}

	if _, cmd := m.editModeUpdate(tea.KeyPressMsg{Code: '1', Text: "1"}); cmd == nil {
		t.Fatal("1 did not react")
	}

	m, _ = m.editModeUpdate(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.state.chat.edit.active || m.state.chat.social.replyTo != "m1" {
		t.Fatalf("reply: active=%v replyTo=%q", m.state.chat.edit.active, m.state.chat.social.replyTo)
	}
	if line := m.chatStatusLine(120); !strings.Contains(line, "hello there") {
		t.Fatalf("status line doesn't show the reply target: %q", line)
	}
	m, _, _ = m.chatKeyUpdate(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state.chat.social.replyTo != "" {
		t.Fatal("esc did not cancel the reply")
	}
}

func TestRepliesReactionsAndReadsRender(t *testing.T) {
	m := newChatModel(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = tm.(model)
	m.state.chat.members.add(apisdk.UserResponse{ID: "friend", Username: "friend"})
	m = withMessages(m,
		apisdk.MessageResponse{ID: "m1", UserID: "friend", Username: "friend", Content: "original words", CreatedAt: time.Now()},
		apisdk.MessageResponse{ID: "m2", UserID: m.userID, Username: "me", Content: "answer", ReplyToID: "m1", CreatedAt: time.Now(),
			Reactions: []apisdk.Reaction{{UserID: "friend", Emoji: "🎉"}, {UserID: "x", Emoji: "🎉"}}},
	)
	m, _ = m.chatUpdate(readUpdateMsg{userID: "friend", messageID: "m2"})

	out := m.renderMessages()
	for _, want := range []string{"↳ friend: original words", "🎉 2", "seen by friend"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered chat lacks %q", want)
		}
	}
}

func TestTypingIndicatorExpires(t *testing.T) {
	m := newChatModel(t)
	m, _ = m.chatUpdate(memberTypingMsg{userID: "friend", username: "friend"})
	if got := m.state.chat.social.typingText(); got != "friend is typing…" {
		t.Fatalf("typing = %q", got)
	}
	m, _ = m.chatUpdate(memberTypingMsg{userID: m.userID, username: "me"})
	if got := m.state.chat.social.typingText(); got != "friend is typing…" {
		t.Fatalf("own typing shown: %q", got)
	}
	m.state.chat.social.typing["friend"] = typingMember{name: "friend", until: time.Now().Add(-time.Second)}
	m, _ = m.chatUpdate(typingExpiredMsg{session: m.state.chat.session})
	if got := m.state.chat.social.typingText(); got != "" {
		t.Fatalf("typing after expiry = %q", got)
	}
}

func TestPresenceMarksOfflineMembers(t *testing.T) {
	m := newChatModel(t)
	m.state.chat.members.add(apisdk.UserResponse{ID: "friend", Username: "friend"})
	if out := m.membersPanelView(30, 20); strings.Contains(out, "○") {
		t.Fatal("members shown offline before presence is known")
	}
	m, _ = m.chatUpdate(presenceMsg{online: []string{m.userID}})
	if out := m.membersPanelView(30, 20); !strings.Contains(out, "○") {
		t.Fatal("offline member not marked")
	}
}

func TestMessageSearchFindsLoadedMessages(t *testing.T) {
	m := newChatModel(t)
	m = withMessages(m,
		apisdk.MessageResponse{ID: "1", Username: "a", Content: "Pizza tonight?"},
		apisdk.MessageResponse{ID: "2", Username: "b", Content: "no"},
		apisdk.MessageResponse{ID: "3", Username: "a", Content: "pizza it is"},
	)
	m, _ = m.openMessageSearch()
	for _, r := range "PIZZA" {
		m, _ = m.messageSearchKeyUpdate(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	s := m.state.chat.social.search
	if len(s.matches) != 2 || s.current() != 2 {
		t.Fatalf("matches = %v, current = %d", s.matches, s.current())
	}
	m, _ = m.messageSearchKeyUpdate(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.state.chat.social.search.current(); got != 0 {
		t.Fatalf("after up, current = %d, want 0", got)
	}
	m, _ = m.messageSearchKeyUpdate(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state.chat.social.search.active {
		t.Fatal("esc did not close the search")
	}
}
