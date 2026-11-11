package repository

import (
	"testing"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
)

func TestMessagesAfterExcludesTheAfterMessage(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }

	// Same-second members come back from Redis ordered by member bytes, not time.
	boundary := []*model.Message{
		{ID: "c", CreatedAt: at(900)},
		{ID: "a", CreatedAt: at(100)},
		{ID: "b", CreatedAt: at(500)},
	}
	rest := []*model.Message{
		{ID: "d", CreatedAt: at(1200)},
		{ID: "e", CreatedAt: at(2300)},
	}

	// Polling with the timestamp of "b" must not return "a" or "b" again.
	got := messagesAfter(boundary, rest, at(500), 0)
	assertIDs(t, got, "c", "d", "e")

	got = messagesAfter(boundary, rest, at(500), 2)
	assertIDs(t, got, "c", "d")
}

func TestSortChronologically(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{ID: "z", CreatedAt: base.Add(3 * time.Millisecond)},
		{ID: "x", CreatedAt: base.Add(1 * time.Millisecond)},
		{ID: "y", CreatedAt: base.Add(2 * time.Millisecond)},
	}
	sortChronologically(msgs)
	assertIDs(t, msgs, "x", "y", "z")
}

func assertIDs(t *testing.T, msgs []*model.Message, want ...string) {
	t.Helper()
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d (%v)", len(msgs), len(want), want)
	}
	for i, m := range msgs {
		if m.ID != want[i] {
			t.Fatalf("message %d: got %q, want %q", i, m.ID, want[i])
		}
	}
}
