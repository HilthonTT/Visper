package qr

import (
	"strings"
	"testing"

	charmansi "github.com/charmbracelet/x/ansi"
)

func TestGenerate(t *testing.T) {
	tests := []string{"a", "https://example.com/rooms/join?code=ABC123&token=xyz"}
	for _, text := range tests {
		out, size, err := Generate(text)
		if err != nil {
			t.Fatalf("Generate(%q): %v", text, err)
		}
		if size < 21 || size%2 == 0 {
			t.Fatalf("unexpected QR size %d", size)
		}
		lines := strings.Split(strings.TrimSuffix(charmansi.Strip(out), "\n"), "\n")
		// ceil(size/2) rows of modules plus the trailing border row. Dropping
		// the last (odd) module row would make the code unscannable.
		if want := (size+1)/2 + 1; len(lines) != want {
			t.Fatalf("got %d lines, want %d", len(lines), want)
		}
		for i, l := range lines[:len(lines)-1] {
			if w := charmansi.StringWidth(l); w != size {
				t.Fatalf("line %d width = %d, want %d", i, w, size)
			}
		}
	}
}
