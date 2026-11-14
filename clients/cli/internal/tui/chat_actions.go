package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	"github.com/hilthontt/visper/clients/cli/internal/tui/validate"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

// The API rejects message content longer than maxMessageLen characters. For
// encrypted rooms that limit applies to the base64 ciphertext (24-byte nonce +
// 16-byte MAC + sealed envelope), which caps the envelope (the text plus its
// sender and kind, see apisdk.SealMessage) at 710 bytes.
const (
	maxMessageLen            = 1000
	maxEncryptedMessageBytes = apisdk.MaxSealedBytes
)

// Default style and tone of the AI rewrite.
const (
	aiRewriteStyle = "professional"
	aiRewriteTone  = "polite"
)

// Results of the chat's background work. Each carries the chat session it was
// started in, so a result that arrives after leaving the room isn't applied
// to another one.
type (
	aiRewriteResultMsg struct {
		rewritten string
		err       error
		session   context.Context
	}
	uploadResultMsg struct {
		fileURL string
		err     error
		session context.Context
	}
)

// roomRequest is apiRequest for a call on the current room; it does nothing
// outside a room.
func (m model) roomRequest(action string, fn func(ctx context.Context, client *apisdk.Client, roomID string, opts []option.RequestOption) error) tea.Cmd {
	if m.state.chat.room == nil {
		return nil
	}
	client, roomID, session := m.client, m.state.chat.room.ID, m.state.chat.session
	return m.apiRequest(action, func(ctx context.Context, opts []option.RequestOption) error {
		// The SDK encrypts with the key it holds when the request runs, and
		// leaving the room (or entering another) changes that key. Tie the
		// request to the room's session so it is never sent once the room is
		// left: it would go out unencrypted, or sealed with the wrong key.
		if session != nil {
			if session.Err() != nil {
				return nil
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			stop := context.AfterFunc(session, cancel)
			defer stop()
		}
		return fn(ctx, client, roomID, opts)
	})
}

// messageLength returns the draft's length and the limit it is measured
// against (bytes for encrypted rooms, characters otherwise).
func (m model) messageLength() (int, int) {
	v := m.state.chat.messageInput.Value()
	if m.state.chat.encrypted() {
		return apisdk.SealedMessageSize(m.userID, "", v), maxEncryptedMessageBytes
	}
	return len([]rune(v)), maxMessageLen
}

// checkMessageLength validates content (a new message, or an edit of
// messageID) against the room's message limit and, when it is too long,
// returns the text to tell the user.
func (m model) checkMessageLength(content, messageID string) (string, bool) {
	if err := validate.NotEmpty("message")(content); err != nil {
		return "Messages can't be empty", false
	}
	if m.state.chat.encrypted() {
		if apisdk.SealedMessageSize(m.userID, messageID, content) > maxEncryptedMessageBytes {
			budget := maxEncryptedMessageBytes - apisdk.SealedMessageSize(m.userID, messageID, "")
			return fmt.Sprintf("Encrypted messages can be up to about %d bytes", budget), false
		}
		return "", true
	}
	if err := validate.WithinLen(1, maxMessageLen, "message")(content); err != nil {
		return fmt.Sprintf("Messages can be up to %d characters", maxMessageLen), false
	}
	return "", true
}

func (m model) sendMessage() (model, tea.Cmd) {
	content := m.state.chat.messageInput.Value()
	if content == "" {
		return m, nil
	}

	if text, ok := m.checkMessageLength(content, ""); !ok {
		return m.toast(theme.ToneWarning, text)
	}

	replyTo := m.state.chat.social.replyTo
	m.state.chat.social.replyTo = ""
	m.state.chat.messageInput.SetValue("")
	m.state.chat.viewport.GotoBottom()
	m.state.chat.unread = 0
	return m, m.sendContent("send message", content, replyTo)
}

// sendContent posts a message, answering replyTo if set; the SDK encrypts it
// (and sets the encrypted flag) when the room has a key.
func (m model) sendContent(action, content, replyTo string) tea.Cmd {
	return m.roomRequest(action, func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		_, err := c.Message.Send(ctx, roomID, apisdk.SendMessageParams{Content: content, ReplyToID: replyTo}, opts...)
		return err
	})
}

func (m model) updateMessage(messageID, content string) tea.Cmd {
	return m.roomRequest("update message", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		_, err := c.Message.Update(ctx, roomID, messageID, apisdk.UpdateMessageParams{Content: content}, opts...)
		return err
	})
}

func (m model) deleteMessage(messageID string) tea.Cmd {
	return m.roomRequest("delete message", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		_, err := c.Message.Delete(ctx, roomID, messageID, opts...)
		return err
	})
}

func (m model) kickMember(userID string) tea.Cmd {
	return m.roomRequest("kick member", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		_, err := c.Room.KickMember(ctx, roomID, userID, opts...)
		return err
	})
}

func (m model) generateJoinCode() tea.Cmd {
	return m.roomRequest("generate a new join code", func(ctx context.Context, c *apisdk.Client, roomID string, opts []option.RequestOption) error {
		return c.Room.GenerateNewJoinCode(ctx, roomID, opts...)
	})
}

// rewriteDraft asks the AI to rewrite the draft message.
func (m model) rewriteDraft() (model, tea.Cmd) {
	content := m.state.chat.messageInput.Value()
	if content == "" {
		return m.toast(theme.ToneInfo, "Write a draft first, then ctrl+a rewrites it")
	}
	if m.state.chat.aiEnhancing {
		return m, nil
	}
	m.state.chat.aiEnhancing = true

	client, ctx, opts, session := m.client, m.ctx, m.requestOpts(), m.state.chat.session
	req := apisdk.AIEnhanceRequest{Message: content, Style: aiRewriteStyle, Tone: aiRewriteTone}
	anim := m.restartAnim()
	return m, tea.Batch(anim, func() tea.Msg {
		res, err := client.AI.Enhance(ctx, req, opts...)
		if err != nil {
			return aiRewriteResultMsg{err: err, session: session}
		}
		return aiRewriteResultMsg{rewritten: res.Enhanced, session: session}
	})
}

// uploadImage encrypts and uploads an image file; the result sends its URL as
// a message. The message (encrypted like any other) also carries the file's
// name in the URL fragment, which tells readers it is an image to decrypt.
func (m model) uploadImage(path string) (model, tea.Cmd) {
	if m.state.chat.room == nil {
		return m, nil
	}
	m.state.chat.uploading = true

	client, ctx, roomID, userToken, session := m.client, m.ctx, m.state.chat.room.ID, m.userToken, m.state.chat.session
	key := m.state.chat.roomKey
	anim := m.restartAnim()
	return m, tea.Batch(anim, func() tea.Msg {
		result, err := client.File.UploadEncrypted(ctx, roomID, path, key, userToken)
		if err != nil {
			return uploadResultMsg{err: err, session: session}
		}
		return uploadResultMsg{fileURL: encryptedImageContent(result.URL, filepath.Base(path)), session: session}
	})
}
