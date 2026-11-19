package apisdk

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Identity
//
// An Identity is one running client's keys: a signing key (Ed25519) that
// signs everything it sends, and a device key (X25519) that new room keys are
// sealed to. Both live only as long as the process, like the user token.

// Identity is a client's user ID with its signing and device keys.
type Identity struct {
	userID string
	sign   ed25519.PrivateKey
	device *DeviceKey
}

// NewIdentity returns an identity for userID with a new signing key. device
// is the client's device key; a new one is made if it is nil.
func NewIdentity(userID string, device *DeviceKey) (*Identity, error) {
	if userID == "" {
		return nil, ErrNoSender
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signing key: %w", err)
	}
	if device == nil {
		if device, err = NewDeviceKey(); err != nil {
			return nil, err
		}
	}
	return &Identity{userID: userID, sign: priv, device: device}, nil
}

// UserID returns the identity's public user ID.
func (id *Identity) UserID() string { return id.userID }

// Device returns the identity's device key.
func (id *Identity) Device() *DeviceKey { return id.device }

// SigningPublicKey returns the public signing key, base64url encoded.
func (id *Identity) SigningPublicKey() string {
	return base64.RawURLEncoding.EncodeToString(id.sign.Public().(ed25519.PublicKey))
}

// ErrNoSender is returned when making an identity without a user ID.
var ErrNoSender = errors.New("encrypted rooms need the sender's user ID")

// ---------------------------------------------------------------------------
// Key cards
//
// A key card tells the room which signing and device keys belong to a
// member. It is a signed envelope of kind "c" whose text is
// "<signing public key>.<device public key>" (base64url), signed by that
// signing key and sealed with the room key, so the server (which has no
// room key) can't forge one, and stored by the API under the uploader's user
// ID, so a member can't publish one for someone else. Members upload a card
// when they enter a room and again whenever its key changes.

type memberCard struct {
	sign        ed25519.PublicKey
	device      string // base64url X25519 public key
	fingerprint string
}

// ---------------------------------------------------------------------------
// Fingerprints
//
// The server relays key cards, so on its own a client can't tell a member's
// real card from one the server (helped by someone who has the room key)
// made in their name. Members rule that out by comparing fingerprints over a
// channel they trust: in person, or on a call.
//
//	fingerprint = SHA-512( "visper/keycard/v1\n<user ID>\n" +
//	                       "<signing key>\n<device key>\n" )   (keys base64url)
//
// shown as six groups of five digits: group i is bytes 5i..5i+4 read as a
// big-endian integer, mod 100000. Verifying a member pins the card with that
// fingerprint: from then on only it is accepted for their signatures
// (including the owner's rekeys), the owner's rekeys go only to its device,
// and any other card for them shows as a changed key.

const (
	fingerprintContext = "visper/keycard/v1"
	fingerprintGroups  = 6
)

// KeyFingerprint returns the fingerprint of userID's key card holding
// signKey and deviceKey (both base64url).
func KeyFingerprint(userID, signKey, deviceKey string) string {
	sum := sha512.Sum512([]byte(fingerprintContext + "\n" + userID + "\n" + signKey + "\n" + deviceKey + "\n"))
	groups := make([]string, fingerprintGroups)
	for i := range groups {
		var b [8]byte
		copy(b[3:], sum[i*5:i*5+5])
		groups[i] = fmt.Sprintf("%05d", binary.BigEndian.Uint64(b[:])%100000)
	}
	return strings.Join(groups, " ")
}

// normalizeFingerprint keeps only the digits of a typed or pasted
// fingerprint.
func normalizeFingerprint(fp string) string {
	var b strings.Builder
	for _, r := range fp {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// KeyStatus is whether a member's key card has been verified.
type KeyStatus int

const (
	// KeyUnverified means no fingerprint was compared for the member.
	KeyUnverified KeyStatus = iota
	// KeyVerified means the member's fingerprint was compared and pinned.
	KeyVerified
	// KeyChanged means the member was verified, but a different card has
	// since appeared for them. Only the pinned card is trusted until they
	// are verified again.
	KeyChanged
)

// ErrFingerprintMismatch is returned when verifying a member with a
// fingerprint none of their key cards has.
var ErrFingerprintMismatch = errors.New("the fingerprint doesn't match any of the member's keys")

// ---------------------------------------------------------------------------
// RoomCrypto

// RoomCrypto holds what a client needs to encrypt and verify one room's
// content: its identity, the room's keys (the current one and those it
// replaced, to read older messages), the members' key cards, and what it has
// seen, to catch replays. It is safe for concurrent use.
type RoomCrypto struct {
	me      *Identity
	roomID  string
	ownerID string

	mu       sync.RWMutex
	keys     []string // base64 room keys, oldest first; the last is current
	epoch    int      // the rekey epoch of the current key, as far as we know
	cards    map[string][]memberCard
	nonces   map[string]string    // new-message nonce -> message ID
	versions map[string]time.Time // message ID -> time of the version shown
	departed map[string]bool      // members seen leaving or being kicked
	verified map[string]string    // member -> pinned card's fingerprint
	changed  map[string]bool      // verified members with a new, unpinned card
}

// NewRoomCrypto returns the crypto state for a room entered with keyB64
// (from the invite, or made when creating the room). ownerID is the room's
// owner, the only member whose rekeys are accepted; it should be the owner
// the client saw when it entered the room and never change afterwards.
func NewRoomCrypto(me *Identity, roomID, ownerID, keyB64 string) (*RoomCrypto, error) {
	if me == nil {
		return nil, ErrNoSender
	}
	if _, err := roomKeyBytes(keyB64); err != nil {
		return nil, err
	}
	return &RoomCrypto{
		me:       me,
		roomID:   roomID,
		ownerID:  ownerID,
		keys:     []string{keyB64},
		cards:    map[string][]memberCard{},
		nonces:   map[string]string{},
		versions: map[string]time.Time{},
		departed: map[string]bool{},
		verified: map[string]string{},
		changed:  map[string]bool{},
	}, nil
}

// Identity returns the client's identity.
func (rc *RoomCrypto) Identity() *Identity { return rc.me }

// OwnerID returns the room owner pinned when the room was entered.
func (rc *RoomCrypto) OwnerID() string { return rc.ownerID }

// CurrentKey returns the key new content is encrypted with (base64).
func (rc *RoomCrypto) CurrentKey() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.keys[len(rc.keys)-1]
}

// Epoch returns the rekey epoch of the current key, as far as this client
// knows (0 until it sees a rekey).
func (rc *RoomCrypto) Epoch() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.epoch
}

// ringNewestFirst returns the room keys, current first.
func (rc *RoomCrypto) ringNewestFirst() []string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	ring := slices.Clone(rc.keys)
	slices.Reverse(ring)
	return ring
}

// open decrypts a signed envelope with any of the room's keys.
func (rc *RoomCrypto) open(ciphertext string) (*envelope, []byte, []byte, error) {
	var firstErr error
	for _, key := range rc.ringNewestFirst() {
		env, sig, body, err := openEnvelope(ciphertext, key)
		if err == nil {
			return env, sig, body, nil
		}
		if firstErr == nil || errors.Is(err, ErrUnverified) {
			firstErr = err
		}
	}
	return nil, nil, nil, firstErr
}

// verifySender checks sig over body against the signing keys on from's
// cards; once from is verified, only the pinned card counts.
func (rc *RoomCrypto) verifySender(from string, body, sig []byte) error {
	rc.mu.RLock()
	cards := rc.cards[from]
	pinned, isPinned := rc.verified[from]
	rc.mu.RUnlock()
	if len(cards) == 0 {
		return ErrUnknownSender
	}
	for _, c := range cards {
		if isPinned && c.fingerprint != pinned {
			continue
		}
		if ed25519.Verify(c.sign, body, sig) {
			return nil
		}
	}
	return ErrUnverified
}

// ---------------------------------------------------------------------------
// Cards

// MyCard returns this client's key card, sealed with the current room key,
// to upload with RoomService.PutKeyCard.
func (rc *RoomCrypto) MyCard() (string, error) {
	text := rc.me.SigningPublicKey() + codeSeparator + rc.me.device.PublicKey()
	env := newEnvelope(kindCard, rc.me.userID, "", text, "", time.Now())
	return sealEnvelope(env, rc.me.sign, rc.CurrentKey())
}

// AddCard records a key card the API stores for userID. Cards that don't
// decrypt with the room's keys (sealed with a key this client never had),
// aren't self-signed or name someone else are ignored and reported as an
// error.
func (rc *RoomCrypto) AddCard(userID, sealed string) error {
	env, sig, body, err := rc.open(sealed)
	if err != nil {
		return err
	}
	if env.Kind != kindCard || env.From != userID {
		return ErrUnverified
	}
	signPart, devicePart, ok := strings.Cut(env.Text, codeSeparator)
	if !ok {
		return ErrUnverified
	}
	signKey, err := base64.RawURLEncoding.DecodeString(signPart)
	if err != nil || len(signKey) != ed25519.PublicKeySize || !ed25519.Verify(signKey, body, sig) {
		return ErrUnverified
	}
	if _, err := parsePublicKey(devicePart); err != nil {
		return ErrUnverified
	}

	fp := KeyFingerprint(userID, signPart, devicePart)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, c := range rc.cards[userID] {
		if c.fingerprint == fp {
			return nil
		}
	}
	rc.cards[userID] = append(rc.cards[userID], memberCard{sign: signKey, device: devicePart, fingerprint: fp})
	if pinned, ok := rc.verified[userID]; ok && pinned != fp {
		rc.changed[userID] = true
	}
	return nil
}

// AddCards records every card in cards (user ID -> sealed cards, oldest
// first), as returned by RoomService.GetCryptoState. Cards that can't be
// verified are skipped.
func (rc *RoomCrypto) AddCards(cards map[string][]string) {
	for userID, list := range cards {
		for _, sealed := range list {
			_ = rc.AddCard(userID, sealed)
		}
	}
}

// HasCard reports whether a verified key card is known for userID.
func (rc *RoomCrypto) HasCard(userID string) bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.cards[userID]) > 0
}

// MyFingerprint returns the fingerprint of this client's key card, for
// members to compare with what their client shows for us.
func (rc *RoomCrypto) MyFingerprint() string {
	return KeyFingerprint(rc.me.userID, rc.me.SigningPublicKey(), rc.me.device.PublicKey())
}

// Fingerprint returns the fingerprint to compare for userID: the pinned
// card's while they are verified and nothing changed, otherwise their
// latest card's. It is "" if no card is known for them.
func (rc *RoomCrypto) Fingerprint(userID string) string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if pinned, ok := rc.verified[userID]; ok && !rc.changed[userID] {
		return pinned
	}
	cards := rc.cards[userID]
	if len(cards) == 0 {
		return ""
	}
	return cards[len(cards)-1].fingerprint
}

// VerifyMember pins the card of userID whose fingerprint is fingerprint
// (spaces and other separators are ignored), once the user compared it with
// the member over a channel they trust. It replaces an earlier pin and
// clears a changed key.
func (rc *RoomCrypto) VerifyMember(userID, fingerprint string) error {
	want := normalizeFingerprint(fingerprint)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, c := range rc.cards[userID] {
		if normalizeFingerprint(c.fingerprint) == want {
			rc.verified[userID] = c.fingerprint
			delete(rc.changed, userID)
			return nil
		}
	}
	return ErrFingerprintMismatch
}

// UnverifyMember forgets that userID was verified, so any of their cards is
// accepted again.
func (rc *RoomCrypto) UnverifyMember(userID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.verified, userID)
	delete(rc.changed, userID)
}

// KeyStatus reports whether userID's key card was verified, and whether a
// different one has appeared since.
func (rc *RoomCrypto) KeyStatus(userID string) KeyStatus {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	switch {
	case rc.changed[userID]:
		return KeyChanged
	case rc.verified[userID] != "":
		return KeyVerified
	}
	return KeyUnverified
}

// ---------------------------------------------------------------------------
// Sealing

// SealMessage signs and encrypts the text of a new message. fileHash is the
// FileHash of the encrypted file an image message points to, or "".
func (rc *RoomCrypto) SealMessage(text, fileHash string) (string, error) {
	env := newEnvelope(kindMessage, rc.me.userID, "", text, fileHash, time.Now())
	return sealEnvelope(env, rc.me.sign, rc.CurrentKey())
}

// SealEdit signs and encrypts new text for message messageID.
func (rc *RoomCrypto) SealEdit(messageID, text string) (string, error) {
	env := newEnvelope(kindMessage, rc.me.userID, messageID, text, "", time.Now())
	return sealEnvelope(env, rc.me.sign, rc.CurrentKey())
}

// SealReaction signs and encrypts this client's reaction to messageID.
func (rc *RoomCrypto) SealReaction(messageID, emoji string) (string, error) {
	env := newEnvelope(kindReaction, rc.me.userID, messageID, emoji, "", time.Now())
	return sealEnvelope(env, rc.me.sign, rc.CurrentKey())
}

// SealedSize returns the size of the envelope SealMessage (or, with
// messageID, SealEdit) would make for text. It must not exceed
// MaxSealedBytes.
func (rc *RoomCrypto) SealedSize(messageID, text, fileHash string) int {
	return SealedMessageSize(rc.me.userID, messageID, text, fileHash)
}

// ---------------------------------------------------------------------------
// Opening

// Opened is verified message content.
type Opened struct {
	Text string
	// FileHash is the hash of the encrypted file an image message points
	// to; check downloads with VerifyFile.
	FileHash string
	// Edited reports that the content is an edit of the message.
	Edited bool
}

// OpenMessage decrypts and verifies the content of message messageID, shown
// as sent by from: a new message, or (in history) its latest edit. created
// and updated are when the server says the message was sent and last edited
// (zero if never); the content must have been written around then.
func (rc *RoomCrypto) OpenMessage(ciphertext, from, messageID string, created, updated time.Time) (Opened, error) {
	env, err := rc.openKind(ciphertext, kindMessage, from)
	if err != nil {
		return Opened{}, err
	}
	switch env.Msg {
	case "":
		// A new message needs a send time to check: with none, a server
		// could replay an old message as new just by leaving it out.
		if created.IsZero() || !near(env.time(), created) {
			return Opened{}, ErrReplayed
		}
		if err := rc.claimNonce(env.Nonce, messageID); err != nil {
			return Opened{}, err
		}
	case messageID:
		at := updated
		if at.IsZero() {
			at = created
		}
		if !near(env.time(), at) {
			return Opened{}, ErrReplayed
		}
	default:
		return Opened{}, ErrUnverified
	}
	if err := rc.advance(messageID, env.time()); err != nil {
		return Opened{}, err
	}
	return Opened{Text: env.Text, FileHash: env.FileHash, Edited: env.Msg != ""}, nil
}

// OpenEdit decrypts and verifies an edit of message messageID, written by
// from (the message's author) and received at at. Edits older than the
// version already shown are refused, so the server can't roll one back.
func (rc *RoomCrypto) OpenEdit(ciphertext, from, messageID string, at time.Time) (Opened, error) {
	env, err := rc.openKind(ciphertext, kindMessage, from)
	if err != nil {
		return Opened{}, err
	}
	if env.Msg != messageID {
		return Opened{}, ErrUnverified
	}
	if !at.IsZero() && !near(env.time(), at) {
		return Opened{}, ErrReplayed
	}
	if err := rc.advance(messageID, env.time()); err != nil {
		return Opened{}, err
	}
	return Opened{Text: env.Text, FileHash: env.FileHash, Edited: true}, nil
}

// OpenReaction decrypts and verifies from's reaction to messageID.
func (rc *RoomCrypto) OpenReaction(ciphertext, from, messageID string) (string, error) {
	env, err := rc.openKind(ciphertext, kindReaction, from)
	if err != nil {
		return "", err
	}
	if env.Msg != messageID {
		return "", ErrUnverified
	}
	return env.Text, nil
}

// openKind decrypts an envelope of kind written by from and checks its
// signature.
func (rc *RoomCrypto) openKind(ciphertext, kind, from string) (*envelope, error) {
	env, sig, body, err := rc.open(ciphertext)
	if err != nil {
		return nil, err
	}
	if env.Kind != kind || env.From != from {
		return nil, ErrUnverified
	}
	if err := rc.verifySender(from, body, sig); err != nil {
		return nil, err
	}
	return env, nil
}

// claimNonce records that a new message's nonce belongs to messageID; the
// same envelope under another message ID is a replay.
func (rc *RoomCrypto) claimNonce(nonce, messageID string) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if id, seen := rc.nonces[nonce]; seen && id != messageID {
		return ErrReplayed
	}
	rc.nonces[nonce] = messageID
	return nil
}

// advance records that the version of messageID written at t is shown; an
// older version than one already shown is refused.
func (rc *RoomCrypto) advance(messageID string, t time.Time) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if shown, ok := rc.versions[messageID]; ok && t.Before(shown) {
		return ErrReplayed
	}
	rc.versions[messageID] = t
	return nil
}

// near reports whether an envelope written at written is plausibly the one
// received at at. A zero at (unknown) always passes.
func near(written, at time.Time) bool {
	if at.IsZero() {
		return true
	}
	d := written.Sub(at)
	return d <= MaxClockSkew && d >= -MaxClockSkew
}

// ---------------------------------------------------------------------------
// Files

// FileHash returns the hash an image message carries for an encrypted file:
// the base64url SHA-256 of its bytes.
func FileHash(encrypted []byte) string {
	sum := sha256.Sum256(encrypted)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ErrFileMismatch means a downloaded file is not the one the message names.
var ErrFileMismatch = errors.New("the file does not match its message")

// EncryptFile encrypts a file with the current room key and returns it with
// the hash to send in the message pointing to it.
func (rc *RoomCrypto) EncryptFile(plaintext []byte) ([]byte, string, error) {
	data, err := EncryptFile(plaintext, rc.CurrentKey())
	if err != nil {
		return nil, "", err
	}
	return data, FileHash(data), nil
}

// DecryptFile checks a downloaded file against the hash in its message and
// decrypts it with any of the room's keys. Messages without a hash are
// refused.
func (rc *RoomCrypto) DecryptFile(data []byte, fileHash string) ([]byte, error) {
	if fileHash == "" || FileHash(data) != fileHash {
		return nil, ErrFileMismatch
	}
	lastErr := ErrDecryptionFailed
	for _, key := range rc.ringNewestFirst() {
		plain, err := DecryptFile(data, key)
		if err == nil {
			return plain, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// ---------------------------------------------------------------------------
// Rekeying
//
// When a member leaves or is kicked, the owner makes a new room key, seals it
// to each remaining member's device key (from their card) and signs the lot:
//
//	signature = Ed25519( "visper/rekey/v1\n<room ID>\n<epoch>\n" +
//	                     "<user ID>=<sealed key>\n" for each member, sorted )
//
// The API stores it and relays it to the room. Members check the owner's
// signature, open their sealed key, switch to it and upload a new card. Old
// keys are kept to read older messages; the member who left never gets the
// new one.

const rekeyContext = "visper/rekey/v1"

// Rekey is a signed set of sealed room keys, one per member.
type Rekey struct {
	Epoch     int               `json:"epoch"`
	Keys      map[string]string `json:"keys"`
	Signature string            `json:"signature"`
}

func rekeyMessage(roomID string, epoch int, keys map[string]string) []byte {
	var b strings.Builder
	line := func(parts ...string) {
		for _, p := range parts {
			b.WriteString(p)
		}
		b.WriteByte('\n')
	}
	line(rekeyContext)
	line(roomID)
	line(strconv.Itoa(epoch))
	ids := make([]string, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		line(id, "=", keys[id])
	}
	return []byte(b.String())
}

// MarkDeparted records that userID left or was kicked, so NewRekey leaves
// them out even if the server still lists them.
func (rc *RoomCrypto) MarkDeparted(userID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.departed[userID] = true
}

// ClearDeparted forgets that userID departed, for when they rejoin; without
// it a member who left and came back was silently left out of every later
// rekey.
func (rc *RoomCrypto) ClearDeparted(userID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	delete(rc.departed, userID)
}

// IsOwner reports whether this client owns the room.
func (rc *RoomCrypto) IsOwner() bool { return rc.me.userID == rc.ownerID }

// ErrNotOwner is returned when a non-owner tries to rekey.
var ErrNotOwner = errors.New("only the room owner can change the room key")

// NewRekey makes a new room key for members (the server's member list) and
// returns the signed rekey to post, the new key, and the members that could
// not be given it because they have no key card. Call CommitKey once the API
// accepted the rekey.
func (rc *RoomCrypto) NewRekey(members []string, currentEpoch int) (*Rekey, string, []string, error) {
	if !rc.IsOwner() {
		return nil, "", nil, ErrNotOwner
	}
	newKey, err := NewRoomKey()
	if err != nil {
		return nil, "", nil, err
	}

	rc.mu.RLock()
	var targets []struct{ id, device string }
	var missing []string
	for _, id := range members {
		if id == rc.me.userID || rc.departed[id] {
			continue
		}
		cards := rc.cards[id]
		if len(cards) == 0 {
			missing = append(missing, id)
			continue
		}
		// The latest card is the member's current device; once they are
		// verified, only the pinned card is trusted.
		card := cards[len(cards)-1]
		if pinned, ok := rc.verified[id]; ok {
			for _, c := range cards {
				if c.fingerprint == pinned {
					card = c
				}
			}
		}
		targets = append(targets, struct{ id, device string }{id, card.device})
	}
	rc.mu.RUnlock()

	keys := make(map[string]string, len(targets))
	for _, t := range targets {
		sealed, err := SealRoomKey(newKey, t.device)
		if err != nil {
			return nil, "", nil, err
		}
		keys[t.id] = sealed
	}
	epoch := currentEpoch + 1
	sig := ed25519.Sign(rc.me.sign, rekeyMessage(rc.roomID, epoch, keys))
	return &Rekey{Epoch: epoch, Keys: keys, Signature: base64.RawURLEncoding.EncodeToString(sig)}, newKey, missing, nil
}

// CommitKey makes newKey (from NewRekey) the current key at epoch.
func (rc *RoomCrypto) CommitKey(newKey string, epoch int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.addKeyLocked(newKey)
	rc.epoch = max(rc.epoch, epoch)
}

func (rc *RoomCrypto) addKeyLocked(key string) {
	if i := slices.Index(rc.keys, key); i >= 0 {
		rc.keys = slices.Delete(rc.keys, i, i+1)
	}
	rc.keys = append(rc.keys, key)
}

// ErrBadRekey means a rekey was not signed by the room's owner.
var ErrBadRekey = errors.New("the new room key was not signed by the room owner")

// ApplyRekey checks a rekey from the API and, if it is newer than the
// current key and includes this client, switches to its key. It reports
// whether the key changed; the caller should then upload a new card
// (MyCard). Rekeys this client made itself are applied by CommitKey.
func (rc *RoomCrypto) ApplyRekey(r *Rekey) (bool, error) {
	if r == nil || r.Epoch <= rc.Epoch() || rc.IsOwner() {
		return false, nil
	}
	sig, err := base64.RawURLEncoding.DecodeString(r.Signature)
	if err != nil {
		return false, ErrBadRekey
	}
	if err := rc.verifySender(rc.ownerID, rekeyMessage(rc.roomID, r.Epoch, r.Keys), sig); err != nil {
		return false, ErrBadRekey
	}

	sealed, included := r.Keys[rc.me.userID]
	if !included {
		// A rekey from before we joined (our invite already had the
		// newer key), or one that left us out.
		rc.mu.Lock()
		rc.epoch = max(rc.epoch, r.Epoch)
		rc.mu.Unlock()
		return false, nil
	}
	newKey, err := rc.me.device.OpenRoomKey(sealed)
	if err != nil {
		return false, err
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if r.Epoch <= rc.epoch {
		return false, nil // applied concurrently
	}
	rc.addKeyLocked(newKey)
	rc.epoch = r.Epoch
	return true, nil
}
