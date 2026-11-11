package tui

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A message whose content is an image URL is shown as the image itself.

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
	return m, func() tea.Msg {
		bytes, err := fetchImageBytes(content)
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

// isImageURL reports whether s is an http(s) URL of an image file.
func isImageURL(s string) bool {
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	for _, ext := range imageExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func fetchImageBytes(url string) ([]byte, error) {
	resp, err := imageHTTPClient.Get(url)
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
