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
	rc := selfRoomCrypto(t, "u1")
	now := time.Now().UTC().Truncate(time.Millisecond)
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
			"created_at": now.Format(time.RFC3339Nano),
		})
	}))
	client.Message.SetRoomCrypto(rc)

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
	if !res.CreatedAt.Equal(now) {
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
	var uploaded []byte
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
			uploaded = b
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
	if res.Hash == "" || res.Hash != FileHash(uploaded) {
		t.Fatalf("Hash = %q, want the hash of the uploaded ciphertext", res.Hash)
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
	rc := selfRoomCrypto(t, "u1")
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
	client.Message.SetRoomCrypto(rc)

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
	client.Message.AllowPlaintext(true)
	res, err := client.Message.Send(context.Background(), "r1", SendMessageParams{Content: "yo", ReplyToID: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if body["reply_to_id"] != "m1" || res.ReplyToID != "m1" {
		t.Fatalf("request %v, response %+v", body, res)
	}
}

func TestWritesFailClosedWithoutRoomCrypto(t *testing.T) {
	requests := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))
	ctx := context.Background()

	check := func(when string) {
		t.Helper()
		if _, err := client.Message.Send(ctx, "r1", SendMessageParams{Content: "secret"}); !errors.Is(err, ErrNoRoomCrypto) {
			t.Errorf("%s: Send got %v, want ErrNoRoomCrypto", when, err)
		}
		if _, err := client.Message.Update(ctx, "r1", "m1", UpdateMessageParams{Content: "secret"}); !errors.Is(err, ErrNoRoomCrypto) {
			t.Errorf("%s: Update got %v, want ErrNoRoomCrypto", when, err)
		}
		if _, err := client.Message.SetReaction(ctx, "r1", "m1", "👍"); !errors.Is(err, ErrNoRoomCrypto) {
			t.Errorf("%s: SetReaction got %v, want ErrNoRoomCrypto", when, err)
		}
	}

	check("no room crypto")

	// Keys for another room must not be used (or skipped) for this one.
	client.Message.SetRoomCrypto(selfRoomCrypto(t, "u1"))
	if _, err := client.Message.Send(ctx, "other-room", SendMessageParams{Content: "secret"}); !errors.Is(err, ErrNoRoomCrypto) {
		t.Errorf("Send to another room got %v, want ErrNoRoomCrypto", err)
	}

	// Leaving the room (SetRoomCrypto(nil)) must not fall back to plaintext.
	client.Message.SetRoomCrypto(nil)
	check("after leaving")

	if requests != 0 {
		t.Fatalf("%d requests reached the server", requests)
	}
}

// selfRoomCrypto returns crypto state for userID in a new room, knowing its
// own key card.
func selfRoomCrypto(t *testing.T, userID string) *RoomCrypto {
	t.Helper()
	id, err := NewIdentity(userID, nil)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := NewRoomKey()
	rc, err := NewRoomCrypto(id, "r1", userID, key)
	if err != nil {
		t.Fatal(err)
	}
	card, err := rc.MyCard()
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.AddCard(userID, card); err != nil {
		t.Fatal(err)
	}
	return rc
}

func TestGetCryptoStateDecodes(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/r1/crypto" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"key_epoch":2,"rekey_needed":true,
			"rekey":{"epoch":2,"keys":{"u2":"c2VhbGVk"},"signature":"c2ln"},
			"cards":{"u1":["a","b"],"u2":["c"]},"members":["u1","u2"]}`)
	}))
	state, err := client.Room.GetCryptoState(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if state.KeyEpoch != 2 || !state.RekeyNeeded || len(state.Members) != 2 {
		t.Fatalf("state = %+v", state)
	}
	if state.Rekey == nil || state.Rekey.Epoch != 2 || state.Rekey.Keys["u2"] != "c2VhbGVk" || state.Rekey.Signature != "c2ln" {
		t.Fatalf("rekey = %+v", state.Rekey)
	}
	if len(state.Cards["u1"]) != 2 || state.Cards["u2"][0] != "c" {
		t.Fatalf("cards = %+v", state.Cards)
	}
}

func TestPostRekeySendsTheRekey(t *testing.T) {
	var body map[string]any
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rooms/r1/crypto/rekey" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"message":"ok"}`)
	}))
	err := client.Room.PostRekey(context.Background(), "r1", Rekey{Epoch: 3, Keys: map[string]string{"u2": "s"}, Signature: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := body["keys"].(map[string]any)
	if body["epoch"] != float64(3) || body["signature"] != "sig" || keys["u2"] != "s" {
		t.Fatalf("body = %v", body)
	}
}

func TestFileListAndDelete(t *testing.T) {
	var gotMethod, gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			io.WriteString(w, `[{"id":"f1","filename":"file.enc","size":42,"url":"http://x/api/v1/d/r1/f1.enc","createdAt":"2026-01-02T03:04:05Z","uploader":{"id":"u1","username":"Ann"}}]`)
			return
		}
		io.WriteString(w, `{"message":"file deleted successfully"}`)
	}))

	files, err := client.File.List(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/rooms/r1/files" {
		t.Fatalf("list request = %s %s", gotMethod, gotPath)
	}
	if len(files) != 1 || files[0].ID != "f1" || files[0].Size != 42 || files[0].Uploader.Username != "Ann" {
		t.Fatalf("files = %+v", files)
	}

	if err := client.File.Delete(context.Background(), "r1", "a/b"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/rooms/r1/files/a%2Fb" {
		t.Fatalf("delete request = %s %s", gotMethod, gotPath)
	}
	if err := client.File.Delete(context.Background(), "r1", ""); !errors.Is(err, ErrMissingIDParameter) {
		t.Fatalf("delete without ID: %v", err)
	}
}

func TestSendRefusesAPlaintextResponse(t *testing.T) {
	rc := selfRoomCrypto(t, "u1")
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"m1","user_id":"u1","content":"injected by the server","encrypted":false,"created_at":"2026-01-01T00:00:00Z"}`)
	}))
	client.Message.SetRoomCrypto(rc)
	res, err := client.Message.Send(context.Background(), "r1", SendMessageParams{Content: "hello"})
	if !errors.Is(err, ErrUnverified) || res.Content == "injected by the server" {
		t.Fatalf("Send = %+v, %v", res, err)
	}
	upd, err := client.Message.Update(context.Background(), "r1", "m1", UpdateMessageParams{Content: "hello"})
	if !errors.Is(err, ErrUnverified) || upd.Content == "injected by the server" {
		t.Fatalf("Update = %+v, %v", upd, err)
	}
}

func TestUserTokenDoesNotFollowRedirectsToAnotherHost(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get(UserTokenHeader)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r1"}`)
	}))
	t.Cleanup(other.Close)
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusFound)
	}))
	if _, err := client.Room.Get(context.Background(), "r1", option.WithUserToken("secret-token")); err != nil {
		t.Fatal(err)
	}
	if leaked != "" {
		t.Fatalf("token sent to another host: %q", leaked)
	}
}
