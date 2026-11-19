package apisdk

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testdata/crypto-vectors.json holds content sealed by this SDK with fixed
// keys. The web client's tests (clients/web/src/lib/room-crypto.test.ts) open
// the same file, so the two implementations can't drift apart. Regenerate it
// with:
//
//	go test -run TestCryptoVectors -update-vectors
var updateVectors = flag.Bool("update-vectors", false, "rewrite testdata/crypto-vectors.json")

const vectorsPath = "testdata/crypto-vectors.json"

type vectorIdentity struct {
	UserID        string `json:"user_id"`
	SignSeed      string `json:"sign_seed"`      // base64url, 32 bytes
	DevicePrivate string `json:"device_private"` // base64url, 32 bytes
	SignPublic    string `json:"sign_public"`
	DevicePublic  string `json:"device_public"`
}

type vectorMessage struct {
	Ciphertext string `json:"ciphertext"`
	From       string `json:"from"`
	MessageID  string `json:"message_id"`
	At         int64  `json:"at"` // unix ms the server received it
	Text       string `json:"text"`
	FileHash   string `json:"file_hash,omitempty"`
}

type cryptoVectors struct {
	RoomID    string         `json:"room_id"`
	RoomKey   string         `json:"room_key"` // base64
	Alice     vectorIdentity `json:"alice"`    // owns the room
	Bob       vectorIdentity `json:"bob"`
	AliceCard string         `json:"alice_card"`
	BobCard   string         `json:"bob_card"`

	Message  vectorMessage `json:"message"`
	Edit     vectorMessage `json:"edit"`
	Reaction vectorMessage `json:"reaction"`
	Image    vectorMessage `json:"image"`

	File struct {
		Plaintext string `json:"plaintext"` // base64
		Encrypted string `json:"encrypted"` // base64
		Hash      string `json:"hash"`
	} `json:"file"`

	Rekey        Rekey  `json:"rekey"`
	RekeyMessage string `json:"rekey_message"` // what the owner signed
	NewKey       string `json:"new_key"`       // base64; bob's sealed key opens to it

	AliceFingerprint string `json:"alice_fingerprint"`
	BobFingerprint   string `json:"bob_fingerprint"`
}

func fixedIdentity(t *testing.T, userID string, seedByte byte) (*Identity, vectorIdentity) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	devicePriv := make([]byte, 32)
	for i := range seed {
		seed[i] = seedByte
		devicePriv[i] = seedByte + 1
	}
	priv, err := ecdh.X25519().NewPrivateKey(devicePriv)
	if err != nil {
		t.Fatal(err)
	}
	id := &Identity{userID: userID, sign: ed25519.NewKeyFromSeed(seed), device: &DeviceKey{priv: priv}}
	b64 := base64.RawURLEncoding.EncodeToString
	return id, vectorIdentity{
		UserID:        userID,
		SignSeed:      b64(seed),
		DevicePrivate: b64(devicePriv),
		SignPublic:    id.SigningPublicKey(),
		DevicePublic:  id.device.PublicKey(),
	}
}

func generateVectors(t *testing.T) cryptoVectors {
	t.Helper()
	var v cryptoVectors
	v.RoomID = "6f1c3c7e-5b1a-4d3e-9f0a-1b2c3d4e5f60"
	v.RoomKey = base64.StdEncoding.EncodeToString([]byte("visper-test-room-key-32-bytes!!!"))
	alice, aliceV := fixedIdentity(t, "0d5f8a7e-1c2b-8a3d-9e4f-5a6b7c8d9e0f", 1)
	bob, bobV := fixedIdentity(t, "9b8a7c6d-5e4f-8a3b-8c2d-1e0f9a8b7c6d", 2)
	v.Alice, v.Bob = aliceV, bobV

	arc, err := NewRoomCrypto(alice, v.RoomID, alice.userID, v.RoomKey)
	if err != nil {
		t.Fatal(err)
	}
	brc, _ := NewRoomCrypto(bob, v.RoomID, alice.userID, v.RoomKey)
	if v.AliceCard, err = arc.MyCard(); err != nil {
		t.Fatal(err)
	}
	v.BobCard, _ = brc.MyCard()
	_ = arc.AddCard(bob.userID, v.BobCard)

	now := time.Now()
	at := now.UnixMilli()
	msg := func(ct, from, id, text, hash string) vectorMessage {
		return vectorMessage{Ciphertext: ct, From: from, MessageID: id, At: at, Text: text, FileHash: hash}
	}
	ct, _ := arc.SealMessage("hello from Go 👋", "")
	v.Message = msg(ct, alice.userID, "m1", "hello from Go 👋", "")
	ct, _ = arc.SealEdit("m1", "edited in Go")
	v.Edit = msg(ct, alice.userID, "m1", "edited in Go", "")
	ct, _ = brc.SealReaction("m1", "🎉")
	v.Reaction = msg(ct, bob.userID, "m1", "🎉", "")

	plain := []byte("not really a PNG")
	enc, hash, _ := arc.EncryptFile(plain)
	v.File.Plaintext = base64.StdEncoding.EncodeToString(plain)
	v.File.Encrypted = base64.StdEncoding.EncodeToString(enc)
	v.File.Hash = hash
	imageText := "https://visper.example/files/abc.enc#cat%20pic.png"
	ct, _ = arc.SealMessage(imageText, hash)
	v.Image = msg(ct, alice.userID, "m2", imageText, hash)

	rekey, newKey, _, err := arc.NewRekey([]string{alice.userID, bob.userID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v.Rekey, v.NewKey = *rekey, newKey
	v.RekeyMessage = string(rekeyMessage(v.RoomID, rekey.Epoch, rekey.Keys))
	v.AliceFingerprint, v.BobFingerprint = arc.MyFingerprint(), brc.MyFingerprint()
	return v
}

// checkVectors opens every vector as bob would.
func checkVectors(t *testing.T, v cryptoVectors) {
	t.Helper()
	bob, _ := fixedIdentity(t, v.Bob.UserID, 2)
	if bob.SigningPublicKey() != v.Bob.SignPublic || bob.device.PublicKey() != v.Bob.DevicePublic {
		t.Fatal("bob's fixed keys don't match the vectors")
	}
	rc, err := NewRoomCrypto(bob, v.RoomID, v.Alice.UserID, v.RoomKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.AddCard(v.Alice.UserID, v.AliceCard); err != nil {
		t.Fatalf("alice's card: %v", err)
	}
	if err := rc.AddCard(v.Bob.UserID, v.BobCard); err != nil {
		t.Fatalf("bob's card: %v", err)
	}
	if rc.MyFingerprint() != v.BobFingerprint || rc.Fingerprint(v.Alice.UserID) != v.AliceFingerprint {
		t.Fatalf("fingerprints = %q, %q", rc.MyFingerprint(), rc.Fingerprint(v.Alice.UserID))
	}

	at := func(m vectorMessage) time.Time { return time.UnixMilli(m.At) }
	got, err := rc.OpenMessage(v.Message.Ciphertext, v.Message.From, v.Message.MessageID, at(v.Message), time.Time{})
	if err != nil || got.Text != v.Message.Text {
		t.Fatalf("message = %+v, %v", got, err)
	}
	got, err = rc.OpenEdit(v.Edit.Ciphertext, v.Edit.From, v.Edit.MessageID, at(v.Edit))
	if err != nil || got.Text != v.Edit.Text {
		t.Fatalf("edit = %+v, %v", got, err)
	}
	emoji, err := rc.OpenReaction(v.Reaction.Ciphertext, v.Reaction.From, v.Reaction.MessageID)
	if err != nil || emoji != v.Reaction.Text {
		t.Fatalf("reaction = %q, %v", emoji, err)
	}
	got, err = rc.OpenMessage(v.Image.Ciphertext, v.Image.From, v.Image.MessageID, at(v.Image), time.Time{})
	if err != nil || got.FileHash != v.File.Hash {
		t.Fatalf("image = %+v, %v", got, err)
	}
	enc, _ := base64.StdEncoding.DecodeString(v.File.Encrypted)
	plain, err := rc.DecryptFile(enc, v.File.Hash)
	if err != nil || base64.StdEncoding.EncodeToString(plain) != v.File.Plaintext {
		t.Fatalf("file = %q, %v", plain, err)
	}
	if string(rekeyMessage(v.RoomID, v.Rekey.Epoch, v.Rekey.Keys)) != v.RekeyMessage {
		t.Fatal("rekey message differs")
	}
	if changed, err := rc.ApplyRekey(&v.Rekey); err != nil || !changed || rc.CurrentKey() != v.NewKey {
		t.Fatalf("rekey = %v, %v", changed, err)
	}
}

func TestCryptoVectors(t *testing.T) {
	if *updateVectors {
		v := generateVectors(t)
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(vectorsPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorsPath, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("%v (generate it with -update-vectors)", err)
	}
	var v cryptoVectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	checkVectors(t, v)
}
