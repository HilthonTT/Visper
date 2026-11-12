package websocket

type NotifySelfRoomInviteRequest struct {
	UserID string `json:"user_id" binding:"required"`
	// SealedKey is the room key encrypted to the invited device's public key
	// (see the SDK's SealRoomKey). The server relays it but cannot read it.
	SealedKey string `json:"sealed_key" binding:"required,max=256"`
}

// sealedKeySize is the decoded size of a sealed room key: an X25519 public
// key, an AES-GCM nonce, and the 32-byte key with its 16-byte tag.
const sealedKeySize = 32 + 12 + 32 + 16
