package strfn

import (
	"strings"
	"testing"

	charmansi "github.com/charmbracelet/x/ansi"
)

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{-5, "0B"},
		{1, "1.00 B"},
		{512, "512.00 B"},
		{999, "999.00 B"},
		{1000, "1.00 kB"},
		{1536, "1.54 kB"},
		{2_500_000, "2.50 MB"},
		{7_429_000_000, "7.43 GB"},
		{1 << 62, "4.61 EB"},
	}
	for _, tt := range tests {
		if got := FormatFileSize(tt.in); got != tt.want {
			t.Errorf("FormatFileSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPlaceOverlay(t *testing.T) {
	bg := strings.Join([]string{
		"..........",
		"..........",
		"..........",
	}, "\n")

	tests := []struct {
		name string
		x, y int
		fg   string
		want string
	}{
		{
			name: "middle",
			x:    2, y: 1, fg: "ab\ncd",
			want: "..........\n..ab......\n..cd......",
		},
		{
			name: "clamped to bottom right",
			x:    20, y: 20, fg: "xy",
			want: "..........\n..........\n........xy",
		},
		{
			name: "fg larger than bg",
			x:    0, y: 0, fg: strings.Repeat("#", 12) + "\n#\n#\n#",
			want: strings.Repeat("#", 12) + "\n#\n#\n#",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := charmansi.Strip(PlaceOverlay(tt.x, tt.y, tt.fg, bg))
			if got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestWhitespaceRenderWidth(t *testing.T) {
	tests := []struct {
		chars string
		width int
	}{
		{"", 5},
		{" ", 3},
		{"ab", 7},
		{"世", 4}, // double-width rune
		{"a世", 5},
	}
	for _, tt := range tests {
		w := whitespace{chars: tt.chars}
		got := charmansi.StringWidth(charmansi.Strip(w.render(tt.width)))
		if got != tt.width {
			t.Errorf("render(%q, %d) has width %d", tt.chars, tt.width, got)
		}
	}
}
