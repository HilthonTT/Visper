package model

// Rekey is a new room key, made by the room's owner when a member leaves or
// is kicked, and sealed by the owner's client to each remaining member's
// device key. The server can't open any of it; it only stores and relays it.
type Rekey struct {
	// Epoch counts the room's rekeys, starting at 1.
	Epoch int `json:"epoch"`
	// Keys maps each member's user ID to the new key sealed to them.
	Keys map[string]string `json:"keys"`
	// Signature is the owner's signature over the epoch and keys, which
	// clients check.
	Signature string `json:"signature"`
}
