package tui

import (
	"testing"
	"time"
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
	}
	for _, tt := range tests {
		if got := isImageURL(tt.in); got != tt.want {
			t.Errorf("isImageURL(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"", 10, ""},
		{"hello world", 20, "hello world"},
		{"hello world", 5, "hello\nworld"},
		{"a b c d", 3, "a b\nc d"},
		{"  spaced   out  ", 20, "spaced out"},
		{"averyveryverylongword x", 5, "averyveryverylongword\nx"},
	}
	for _, tt := range tests {
		if got := wordWrap(tt.in, tt.width); got != tt.want {
			t.Errorf("wordWrap(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
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
