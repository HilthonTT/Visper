package tui

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// A message whose content is an image URL is shown as the image itself.
//
// Images shared in a room are encrypted with the room key (see uploadImage):
// their message is the stored file's URL, ending in ".enc", with the image's
// file name as the fragment, e.g. "https://host/api/v1/d/room/id.enc#cat.png".
// Plain image URLs (e.g. pasted links) are shown too.

const (
	imagePreviewHeight = 15
	// maxImageBytes bounds how much of a remote image is downloaded.
	maxImageBytes = 100 << 20
)

var (
	imageExtensions = []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}
	imageHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// imageCache holds the images of image messages, keyed by message ID (not
// URL, so they are dropped with the room).
type imageCache struct {
	fetched  map[string][]byte
	fetching map[string]bool
	failed   map[string]bool
	// previews are the images rendered for the current layout.
	previews map[string]string
}

type imageFetchedMsg struct {
	messageID string
	bytes     []byte
	err       error
}

func newImageCache() imageCache {
	return imageCache{
		fetched:  make(map[string][]byte),
		fetching: make(map[string]bool),
		failed:   make(map[string]bool),
		previews: make(map[string]string),
	}
}

// fetchImageFor starts downloading the image of msg if it is an image message
// that isn't cached, being fetched or known to be broken.
func (m model) fetchImageFor(messageID, content string) (model, tea.Cmd) {
	c := &m.state.chat.images
	if !isImageURL(content) || c.failed[messageID] || c.fetching[messageID] {
		return m, nil
	}
	if _, ok := c.fetched[messageID]; ok {
		return m, nil
	}
	c.fetching[messageID] = true
	key := m.state.chat.roomKey
	return m, func() tea.Msg {
		bytes, err := fetchImageBytes(content)
		if err == nil && isEncryptedImageURL(content) {
			bytes, err = apisdk.DecryptFile(bytes, key)
		}
		return imageFetchedMsg{messageID: messageID, bytes: bytes, err: err}
	}
}

func (m model) imageFetchedUpdate(msg imageFetchedMsg) model {
	c := &m.state.chat.images
	delete(c.fetching, msg.messageID)
	if msg.err != nil {
		log.Printf("Failed to fetch image for message %s: %v", msg.messageID, msg.err)
		c.failed[msg.messageID] = true
	} else {
		c.fetched[msg.messageID] = msg.bytes
		m = m.renderImagePreview(msg.messageID)
	}

	atBottom := m.state.chat.viewport.AtBottom()
	m = m.refreshMessages()
	if atBottom {
		m.state.chat.viewport.GotoBottom()
	}
	return m
}

// renderImagePreview renders the fetched image for messageID at the current
// width and caches the result.
func (m model) renderImagePreview(messageID string) model {
	c := &m.state.chat.images
	data, ok := c.fetched[messageID]
	if !ok {
		return m
	}
	width := m.chatCenterWidth() - 6
	if width <= 0 {
		return m
	}
	preview, err := m.imagePreviewer.ImagePreviewFromBytes(data, width, imagePreviewHeight, "")
	if err != nil {
		log.Printf("Failed to render image for message %s: %v", messageID, err)
		c.failed[messageID] = true
		return m
	}
	c.previews[messageID] = preview
	return m
}

// rerenderImagePreviews renders every fetched image again for a new layout,
// from the bytes we already have.
func (m model) rerenderImagePreviews() model {
	m.state.chat.images.previews = make(map[string]string)
	for id := range m.state.chat.images.fetched {
		m = m.renderImagePreview(id)
	}
	return m
}

// encryptedImageContent is the message content for an encrypted image stored
// at fileURL.
func encryptedImageContent(fileURL, name string) string {
	return fileURL + "#" + url.PathEscape(name)
}

// isImageURL reports whether s is an http(s) URL of an image file, encrypted
// or not.
func isImageURL(s string) bool {
	return isEncryptedImageURL(s) || isPlainImageURL(s)
}

// isPlainImageURL reports whether s is an http(s) URL ending in an image
// extension.
func isPlainImageURL(s string) bool {
	file, _, _ := strings.Cut(strings.ToLower(s), "#")
	return isHTTPURL(file) && hasImageExtension(file)
}

// isEncryptedImageURL reports whether s is the URL of an encrypted image: an
// encrypted file whose fragment names an image.
func isEncryptedImageURL(s string) bool {
	lower := strings.ToLower(s)
	file, name, ok := strings.Cut(lower, "#")
	return ok && isHTTPURL(file) && strings.HasSuffix(file, encryptedFileExt) && hasImageExtension(name)
}

// encryptedFileExt ends the URLs of files encrypted with the room key.
const encryptedFileExt = ".enc"

func isHTTPURL(lower string) bool {
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func hasImageExtension(lower string) bool {
	for _, ext := range imageExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func fetchImageBytes(rawURL string) ([]byte, error) {
	// The fragment is ours (the file name); never send it.
	rawURL, _, _ = strings.Cut(rawURL, "#")
	resp, err := imageHTTPClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image larger than %d bytes", maxImageBytes)
	}
	return data, nil
}
