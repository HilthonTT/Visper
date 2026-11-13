package apisdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hilthontt/visper/sdk/go/option"
)

func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

func TestMessageListAfterEscapesTimestamp(t *testing.T) {
	var gotQuery string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("timestamp")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"messages":[],"count":0,"room_id":"r1"}`)
	}))

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("", 2*60*60))
	if _, err := client.Message.ListAfter(context.Background(), "r1", MessageListAfterParams{Timestamp: ts}); err != nil {
		t.Fatal(err)
	}
	if want := "2026-01-02T03:04:05+02:00"; gotQuery != want {
		t.Fatalf("timestamp query = %q, want %q", gotQuery, want)
	}
}

func TestMessageSendEncryptsAndDecrypts(t *testing.T) {
	keyB64, _ := GenerateKeyBase64()
	var sent SendMessageParams
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/r1/messages" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "m1", "room_id": "r1", "user_id": "u1", "content": sent.Content, "encrypted": sent.Encrypted,
			"created_at": "2026-01-01T00:00:00Z",
		})
	}))
	client.Message.SetEncryptionKey(keyB64)
	client.Message.SetSender("u1")

	res, err := client.Message.Send(context.Background(), "r1", SendMessageParams{Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !sent.Encrypted || sent.Content == "hello" {
		t.Fatalf("message was not encrypted on the wire: %+v", sent)
	}
	if res.Content != "hello" || res.Encrypted {
		t.Fatalf("response not decrypted: %+v", res)
	}
	if !res.CreatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("CreatedAt = %v", res.CreatedAt)
	}
}

func TestAPIErrorWithNonJSONBody(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "<html>not found</html>")
	}))

	_, err := client.Room.Get(context.Background(), "nope")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *apisdk.Error", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", apiErr.StatusCode)
	}
}

func TestDeleteWithoutResponseBodySucceeds(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":"ok"}`)
	}))
	if err := client.Room.Delete(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestJoinByCodeWithToken(t *testing.T) {
	var body map[string]string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/join-code/secure" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r1","join_code":"ABC123"}`)
	}))
	room, err := client.Room.JoinByCodeWithToken(context.Background(), JoinByCodeWithTokenParams{JoinCode: "ABC123", SecureToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if body["join_code"] != "ABC123" || body["secure_token"] != "tok" {
		t.Fatalf("request body = %v", body)
	}
	if room.ID != "r1" || room.JoinCode != "ABC123" {
		t.Fatalf("room = %+v", room)
	}
	if _, err := client.Room.JoinByCodeWithToken(context.Background(), JoinByCodeWithTokenParams{JoinCode: "x"}); !errors.Is(err, ErrMissingSecureTokenParameter) {
		t.Fatalf("err = %v", err)
	}
}

func TestFileUploadEncryptsContentAndHidesName(t *testing.T) {
	keyB64, _ := NewRoomKey()
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/r1/files/upload" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-User-Token"); got != "u1" {
			t.Errorf("X-User-Token = %q", got)
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
		} else {
			b, _ := io.ReadAll(f)
			if hdr.Filename != "file.enc" {
				t.Errorf("filename sent to the server = %q", hdr.Filename)
			}
			if bytes.Contains(b, []byte("png-bytes")) || !IsEncryptedFile(b) {
				t.Errorf("file was not encrypted: %q", b)
			}
			if plain, err := DecryptFile(b, keyB64); err != nil || string(plain) != "png-bytes" {
				t.Errorf("DecryptFile = %q, %v", plain, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"f1","filename":"file.enc","mimetype":"application/octet-stream","size":9,"url":"http://x/d/r1/f1.enc","createdAt":"2026-01-01T00:00:00Z","uploader":{"id":"u1"}}`)
	}))

	path := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := client.File.UploadEncrypted(context.Background(), "r1", path, keyB64, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != "http://x/d/r1/f1.enc" || res.CreatedAt.IsZero() || res.Uploader.ID != "u1" {
		t.Fatalf("unexpected response %+v", res)
	}
}

func TestClientServicesDoNotShareOptionBackingArray(t *testing.T) {
	c := NewClient(option.WithBaseURL("http://example.invalid"))
	if cap(c.Options) != len(c.Options) {
		t.Fatalf("client options have spare capacity (len %d, cap %d); appends would alias", len(c.Options), cap(c.Options))
	}
}

func TestRoomWebSocketListenStopsOnCancel(t *testing.T) {
	upgrader := websocket.Upgrader{}
	frames := make(chan string, 4)
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/rooms/r1/ws") {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteJSON(WSMessage{Type: MessageReceived, RoomID: "r1", Data: map[string]any{"id": "m1"}})
		// Keep the connection open (silently) until the client goes away.
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case frames <- string(data):
			default:
			}
		}
	}))

	ws, err := client.Room.ConnectWebSocket(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}

	received := make(chan WSMessage, 1)
	ws.SetMessageHandler(func(m WSMessage) { received <- m })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ws.Listen(ctx) }()

	select {
	case m := <-received:
		if m.Type != MessageReceived {
			t.Fatalf("type = %q", m.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message received")
	}

	if err := ws.SendTyping(); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
	if err := ws.SendRead("m1"); err != nil {
		t.Fatalf("SendRead: %v", err)
	}
	for _, want := range []string{`{"type":"typing"}`, `{"messageId":"m1","type":"read"}`} {
		select {
		case got := <-frames:
			if got != want {
				t.Fatalf("frame = %s, want %s", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("frame not received")
		}
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Listen returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not return after context cancellation")
	}

	if err := ws.SendTyping(); !errors.Is(err, ErrWebSocketClosed) {
		t.Fatalf("SendTyping after close = %v, want ErrWebSocketClosed", err)
	}
	if err := ws.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestMessageListAfterKeepsSubSecondPrecision(t *testing.T) {
	var gotQuery string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("timestamp")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"messages":[],"count":0,"room_id":"r1"}`)
	}))

	ts := time.Date(2026, 1, 2, 3, 4, 5, 500_000_000, time.UTC)
	if _, err := client.Message.ListAfter(context.Background(), "r1", MessageListAfterParams{Timestamp: ts}); err != nil {
		t.Fatal(err)
	}
	if want := "2026-01-02T03:04:05.5Z"; gotQuery != want {
		t.Fatalf("timestamp query = %q, want %q", gotQuery, want)
	}
}

func TestPathIDsAreEscaped(t *testing.T) {
	var gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))

	if _, err := client.Room.Get(context.Background(), "a/b?c"); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/rooms/a%2Fb%3Fc"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
	if _, err := client.Message.Delete(context.Background(), "r1", "../x"); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/rooms/r1/messages/..%2Fx"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestWebSocketCloseDoesNotWaitForBlockedWriter(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	upgrader := websocket.Upgrader{}
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-release // never read: the client's writes eventually block
	}))

	ws, err := client.Room.ConnectWebSocket(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		payload := strings.Repeat("x", 1<<20)
		for ws.SendRead(payload) == nil {
		}
	}()
	time.Sleep(500 * time.Millisecond) // let the writer fill the socket buffers and block

	closed := make(chan struct{})
	go func() { ws.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked behind a stalled writer")
	}
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked writer not released by Close")
	}
}

func TestReactionsAreEncryptedAndDecrypted(t *testing.T) {
	keyB64, _ := GenerateKeyBase64()
	var sent SetReactionParams
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/r1/messages/m1/reaction" || r.Method != http.MethodPut {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "m1", "content": "hi", "created_at": "2026-01-01T00:00:00Z",
			"reactions": []map[string]any{{"user_id": "u1", "emoji": sent.Emoji, "encrypted": sent.Encrypted}},
		})
	}))
	client.Message.SetEncryptionKey(keyB64)
	client.Message.SetSender("u1")

	res, err := client.Message.SetReaction(context.Background(), "r1", "m1", "👍")
	if err != nil {
		t.Fatal(err)
	}
	if !sent.Encrypted || sent.Emoji == "👍" {
		t.Fatalf("reaction was not encrypted on the wire: %+v", sent)
	}
	if len(res.Reactions) != 1 || res.Reactions[0].Emoji != "👍" || res.Reactions[0].Encrypted {
		t.Fatalf("reactions not decrypted: %+v", res.Reactions)
	}
}

func TestSendReplyCarriesReplyToID(t *testing.T) {
	var body map[string]any
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"m2","content":"yo","reply_to_id":"m1","created_at":"2026-01-01T00:00:00Z"}`)
	}))
	res, err := client.Message.Send(context.Background(), "r1", SendMessageParams{Content: "yo", ReplyToID: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if body["reply_to_id"] != "m1" || res.ReplyToID != "m1" {
		t.Fatalf("request %v, response %+v", body, res)
	}
}
