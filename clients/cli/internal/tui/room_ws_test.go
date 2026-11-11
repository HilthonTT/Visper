package tui

import (
	"testing"

	apisdk "github.com/hilthontt/visper/sdk/go"
)

func TestParseRoomEvent(t *testing.T) {
	decrypt := func(content string, encrypted bool) string {
		if encrypted {
			return "plain:" + content
		}
		return content
	}

	msg, ok := parseRoomEvent(apisdk.WSMessage{
		Type:   apisdk.MessageReceived,
		RoomID: "r",
		Data: map[string]any{
			"id": "m1", "user_id": "u1", "Username": "alice",
			"content": "cipher", "encrypted": true, "timestamp": "2026-01-02T03:04:05Z",
		},
	}, decrypt)
	got, isMsg := msg.(messageReceivedMsg)
	if !ok || !isMsg || got.message.ID != "m1" || got.message.UserID != "u1" ||
		got.message.Content != "plain:cipher" || got.message.CreatedAt.Year() != 2026 {
		t.Fatalf("message received parsed as %#v, %v", msg, ok)
	}

	if _, ok := parseRoomEvent(apisdk.WSMessage{
		Type: apisdk.MessageReceived,
		Data: map[string]any{"id": "m1"},
	}, decrypt); ok {
		t.Fatal("message without content was accepted")
	}

	msg, _ = parseRoomEvent(apisdk.WSMessage{
		Type: apisdk.MemberList,
		Data: map[string]any{"members": []any{
			map[string]any{"userId": "u1", "username": "alice"},
			map[string]any{"userId": "u2"}, // no name: skipped
			"junk",
		}},
	}, decrypt)
	if list, _ := msg.(memberListMsg); len(list.members) != 1 || list.members[0].Username != "alice" {
		t.Fatalf("member list parsed as %#v", msg)
	}

	msg, _ = parseRoomEvent(apisdk.WSMessage{Type: apisdk.MessageDeleted, Data: apisdk.MessageDeletedPayload{ID: "m9"}}, decrypt)
	if del, _ := msg.(messageDeletedMsg); del.messageID != "m9" {
		t.Fatalf("typed delete payload parsed as %#v", msg)
	}

	msg, _ = parseRoomEvent(apisdk.WSMessage{Type: apisdk.ErrorEvent, Data: map[string]any{}}, decrypt)
	if e, _ := msg.(roomErrorMsg); e.code != "UNKNOWN_ERROR" {
		t.Fatalf("incomplete error parsed as %#v", msg)
	}
}
