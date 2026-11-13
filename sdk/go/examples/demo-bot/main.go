// Command demo-bot is a scripted second participant for recording demos (see
// docs/demos). It creates a room, writes the room's invite code to a file, waits
// for someone to join and then answers each of their messages with the next
// line of its script. It is also a compact end-to-end example of the SDK:
// creating a room, end-to-end encrypted sending and reading messages.
//
//	go run ./examples/demo-bot -code-file /tmp/visper-code
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

// greeting is sent when the guest joins; script answers their messages in order.
const greeting = "hey! glad you made it"

var script = []string{
	"yep, end-to-end encrypted. the server only ever sees ciphertext",
	"the room deletes itself when it expires. no accounts, no traces",
	"see you around!",
}

func main() {
	codeFile := flag.String("code-file", "", "file to write the room's invite code (join code and key) to")
	userToken := flag.String("user-token", "", "private user token the bot identifies with (default: a new random one)")
	timeout := flag.Duration("timeout", 10*time.Minute, "give up after this long")
	silent := flag.Bool("silent", false, "join the conversation as a quiet member: never reply")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, *timeout)
	defer cancelTimeout()

	// Polls every 1.5 seconds: the API rate-limits per client IP (60 req/min for the API as a whole), and
	// the bot usually shares that IP with the client being recorded.
	// VISPER_BASE_URL is read by NewClient (default http://localhost:5005).
	if *userToken == "" {
		*userToken = apisdk.NewUserToken()
	}
	client := apisdk.NewClient(option.WithUserToken(*userToken))
	userID := apisdk.UserIDFromToken(*userToken) // the bot's public ID

	// The room key never leaves the clients: the server only stores
	// ciphertext. Guests get it inside the invite code.
	key, err := apisdk.NewRoomKey()
	if err != nil {
		log.Fatalf("create room key: %v", err)
	}
	room, err := client.Room.Create(ctx, apisdk.RoomCreateParams{ExpiryHours: 1})
	if err != nil {
		log.Fatalf("create room: %v", err)
	}
	client.Message.SetEncryptionKey(key)
	client.Message.SetSender(userID)
	inviteCode, err := apisdk.FormatInviteCode(room.JoinCode, key)
	if err != nil {
		log.Fatalf("format invite code: %v", err)
	}
	log.Printf("room %s created, join code %s", room.ID, room.JoinCode)

	if *codeFile != "" {
		if err := os.WriteFile(*codeFile, []byte(inviteCode), 0o600); err != nil {
			log.Fatalf("write code file: %v", err)
		}
	}

	if err := waitForGuest(ctx, client, room.ID, userID); err != nil {
		log.Fatalf("waiting for a guest: %v", err)
	}
	log.Print("guest joined")

	if *silent {
		<-ctx.Done()
		return
	}

	// The client shows only messages that arrive after its websocket connects
	// (there is no history replay), so give it time to finish joining.
	_ = sleep(ctx, 3*time.Second)
	if _, err := client.Message.Send(ctx, room.ID, apisdk.SendMessageParams{Content: greeting}); err != nil {
		log.Printf("send greeting: %v", err)
	}

	answered := map[string]bool{}
	for line := 0; line < len(script); {
		if err := sleep(ctx, 1500*time.Millisecond); err != nil {
			log.Fatalf("stopped: %v", err)
		}
		msgs, err := client.Message.List(ctx, room.ID, apisdk.MessageListParams{Limit: 50})
		if err != nil {
			log.Printf("list messages: %v", err)
			continue
		}
		for _, m := range msgs.Messages {
			if m.UserID == userID || answered[m.ID] || line >= len(script) {
				continue
			}
			answered[m.ID] = true
			_ = sleep(ctx, 800*time.Millisecond) // "typing"
			if _, err := client.Message.Send(ctx, room.ID, apisdk.SendMessageParams{Content: script[line]}); err != nil {
				log.Printf("send: %v", err)
			}
			line++
		}
	}

	// Stay in the room until the recording ends so the guest doesn't see it vanish.
	<-ctx.Done()
}

func waitForGuest(ctx context.Context, client *apisdk.Client, roomID, self string) error {
	for {
		room, err := client.Room.Get(ctx, roomID)
		if err == nil {
			for _, m := range room.Members {
				if m.ID != self {
					return nil
				}
			}
		}
		if err := sleep(ctx, 1500*time.Millisecond); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
