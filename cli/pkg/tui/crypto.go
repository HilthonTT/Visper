package tui

import (
	"log"

	apisdk "github.com/hilthontt/visper/api-sdk"
)

func (m model) decryptContent(content string, encrypted bool) string {
	if !encrypted {
		return content
	}

	if m.state.chat.room == nil || m.state.chat.room.EncryptionKey == "" {
		log.Printf("Cannot decrypt: no encryption key available")
		return "[Decryption failed: no key]"
	}

	decrypted, err := apisdk.DecryptWithKeyB64(content, m.state.chat.room.EncryptionKey)
	if err != nil {
		log.Printf("Failed to decrypt message: %v", err)
		return "[Decryption failed]"
	}

	return decrypted
}
