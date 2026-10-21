package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hilthontt/visper/api/domain/model"
)

func TestOriginChecker(t *testing.T) {
	tests := []struct {
		name    string
		allowed string
		origin  string
		host    string
		want    bool
	}{
		{"wildcard", "*", "https://evil.example", "api.example", true},
		{"no origin header (CLI)", "https://app.example", "", "api.example", true},
		{"listed origin", "https://app.example, http://localhost:3000", "http://localhost:3000", "api.example", true},
		{"listed origin case/slash insensitive", "https://App.example/", "https://app.example", "api.example", true},
		{"same origin", "https://app.example", "https://api.example", "api.example", true},
		{"foreign origin", "https://app.example", "https://evil.example", "api.example", false},
		{"empty allow list foreign origin", "", "https://evil.example", "api.example", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewOriginChecker(tt.allowed)
			r := httptest.NewRequest(http.MethodGet, "/ws", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := check(r); got != tt.want {
				t.Fatalf("check(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

// newTestClient returns a client without a network connection; it is enough
// to exercise the hub, which only touches the message queue and lifecycle.
func newTestClient(id, roomID string) *Client {
	return &Client{
		Message: make(chan *WSMessage, clientSendBuffer),
		ID:      id,
		RoomID:  roomID,
		closed:  make(chan struct{}),
		drain:   make(chan struct{}),
	}
}

func TestRoomManagerTracksConnectionsPerClient(t *testing.T) {
	rm := NewRoomManager(nil)

	tab1 := newTestClient("alice", "room")
	tab2 := newTestClient("alice", "room")
	bob := newTestClient("bob", "room")

	rm.AddClient(tab1)
	rm.AddClient(tab2)
	rm.AddClient(bob)

	if got := rm.ClientCount("room"); got != 3 {
		t.Fatalf("ClientCount = %d, want 3 (both tabs of the same user)", got)
	}

	rm.RemoveClient(tab1)
	if got := rm.ClientCount("room"); got != 2 {
		t.Fatalf("after RemoveClient ClientCount = %d, want 2", got)
	}
	if !tab1.IsClosed() {
		t.Fatal("removed client should be closed")
	}

	// Sending to a closed client must not panic.
	if err := rm.BroadcastToRoom(&WSMessage{RoomID: "room", Type: MessageReceived}); err != nil {
		t.Fatalf("BroadcastToRoom: %v", err)
	}
	if tab1.send(&WSMessage{}) {
		t.Fatal("send to a closed client should report false")
	}

	rm.DisconnectUser("room", "alice")
	if got := rm.ClientCount("room"); got != 1 {
		t.Fatalf("after DisconnectUser ClientCount = %d, want 1", got)
	}
	if !tab2.isDraining() {
		t.Fatal("disconnected user's client should be draining")
	}

	rm.CloseRoom("room")
	if got := rm.ClientCount("room"); got != 0 {
		t.Fatalf("after CloseRoom ClientCount = %d, want 0", got)
	}
	if !bob.isDraining() {
		t.Fatal("clients of a closed room should be draining")
	}

	if err := rm.BroadcastToRoom(&WSMessage{RoomID: "room"}); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("BroadcastToRoom on empty room = %v, want ErrRoomNotFound", err)
	}
}

type stubMessageRepo struct {
	history []*model.Message
}

func (s *stubMessageRepo) GetByID(context.Context, string, string) (*model.Message, error) {
	return nil, nil
}
func (s *stubMessageRepo) Update(context.Context, *model.Message) error               { return nil }
func (s *stubMessageRepo) Delete(context.Context, string, string) error               { return nil }
func (s *stubMessageRepo) Create(context.Context, *model.Message) error               { return nil }
func (s *stubMessageRepo) Count(context.Context, string) (int64, error)               { return 0, nil }
func (s *stubMessageRepo) DeleteOldMessages(context.Context, string, time.Time) error { return nil }
func (s *stubMessageRepo) GetByRoom(context.Context, string, int64) ([]*model.Message, error) {
	return s.history, nil
}
func (s *stubMessageRepo) GetByRoomAfter(context.Context, string, time.Time, int64) ([]*model.Message, error) {
	return nil, nil
}

func receive(t *testing.T, cl *Client) *WSMessage {
	t.Helper()
	select {
	case msg := <-cl.Message:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatalf("client %s: timed out waiting for a message", cl.ID)
		return nil
	}
}

func TestCoreHistoryBroadcastAndKick(t *testing.T) {
	repo := &stubMessageRepo{history: []*model.Message{
		{ID: "m1", Content: "hello", UserID: "bob", Username: "Bob", CreatedAt: time.Now()},
	}}
	rm := NewRoomManager(nil)
	core := NewCore(rm, repo)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		core.Run(ctx)
		close(done)
	}()

	alice := newTestClient("alice", "room")
	bob := newTestClient("bob", "room")
	if !core.RegisterClient(alice) || !core.RegisterClient(bob) {
		t.Fatal("RegisterClient failed")
	}

	// History is replayed with message IDs.
	for _, cl := range []*Client{alice, bob} {
		hist := receive(t, cl)
		p, ok := hist.Data.(MessagePayload)
		if !ok || p.ID != "m1" {
			t.Fatalf("history message = %#v, want payload with ID m1", hist.Data)
		}
	}

	core.Publish(NewErrorKicked("room", "bob", "Bob", "bye"))

	for _, cl := range []*Client{alice, bob} {
		if msg := receive(t, cl); msg.Type != Kicked {
			t.Fatalf("client %s got %q, want %q", cl.ID, msg.Type, Kicked)
		}
	}

	// The kicked user is removed from the room and their connection drains.
	deadline := time.Now().Add(2 * time.Second)
	for !bob.isDraining() {
		if time.Now().After(deadline) {
			t.Fatal("kicked client was not disconnected")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := rm.ClientCount("room"); got != 1 {
		t.Fatalf("ClientCount after kick = %d, want 1", got)
	}

	cancel()
	<-done

	// After shutdown the hub must not block or panic.
	if core.Publish(&WSMessage{RoomID: "room"}) {
		t.Fatal("Publish after shutdown should report false")
	}
	if core.RegisterClient(newTestClient("carol", "room")) {
		t.Fatal("RegisterClient after shutdown should report false")
	}
	core.UnregisterClient(alice)
	if !alice.IsClosed() {
		t.Fatal("clients should be closed on shutdown")
	}
}
