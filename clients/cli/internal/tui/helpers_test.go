package tui

import (
	"testing"
	"time"

	"github.com/hilthontt/visper/clients/cli/internal/resource"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00"},
		{59 * time.Second, "00:59"},
		{61*time.Second + 400*time.Millisecond, "01:01"},
		{time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{25 * time.Hour, "25:00:00"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.in); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsImageURL(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"https://x/p/a.png", true},
		{"HTTP://X/A.JPEG", true},
		{"http://x/a.webp", true},
		{"ftp://x/a.png", false},
		{"a.png", false},
		{"https://x/a.png?x=1", false},
		{"https://x/a.txt", false},
		{"", false},
		// Encrypted images: the stored file, named by the fragment.
		{"https://x/api/v1/d/r/f.enc#cat.png", true},
		{"https://x/api/v1/d/r/f.enc#My%20Cat.JPG", true},
		{"https://x/api/v1/d/r/f.enc#notes.txt", false},
		{"https://x/api/v1/d/r/f.enc", false},
		{"https://x/a.txt#cat.png", false},
	}
	for _, tt := range tests {
		if got := isImageURL(tt.in); got != tt.want {
			t.Errorf("isImageURL(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestMaxEncryptedMessageBytesFitsServerLimit(t *testing.T) {
	// base64(nonce || secretbox(plaintext)) must not exceed maxMessageLen.
	const nonce, overhead = 24, 16
	n := maxEncryptedMessageBytes + nonce + overhead
	if encoded := (n + 2) / 3 * 4; encoded > maxMessageLen {
		t.Fatalf("encrypted length %d exceeds %d", encoded, maxMessageLen)
	}
	n++
	if encoded := (n + 2) / 3 * 4; encoded <= maxMessageLen {
		t.Fatalf("limit %d is not tight", maxEncryptedMessageBytes)
	}
}

func TestOnlyImagesOnTheAPIHostLoadAutomatically(t *testing.T) {
	saved := resource.Resource.Api.Url
	t.Cleanup(func() { resource.Resource.Api.Url = saved })
	resource.Resource.Api.Url = "https://chat.example.com:5004"

	for in, want := range map[string]bool{
		"https://chat.example.com/api/v1/d/r/f.enc#cat.png":     true,
		"http://CHAT.example.com:5005/api/v1/d/r/f.enc#cat.png": true, // the API behind the proxy
		"https://attacker.example/x.png":                        false,
		"https://attacker.example/api/v1/d/r/f.enc#cat.png":     false,
		"https://chat.example.com.attacker.example/x.png":       false,
		"just text": false,
	} {
		if got := isAutoLoadImage(in); got != want {
			t.Errorf("isAutoLoadImage(%q) = %v, want %v", in, got, want)
		}
	}
}
