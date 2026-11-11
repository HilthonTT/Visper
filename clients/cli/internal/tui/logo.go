package tui

import "strings"

// wordmarkLines spells "visper" in half-block letters.
var wordmarkLines = []string{
	"█ █ █ █▀▀ █▀█ █▀▀ █▀█",
	"▀▄▀ █ ▄▄█ █▀▀ ██▄ █▀▄",
}

// LogoView is the compact, single-line logo: the name and a blinking cursor.
func (m model) LogoView() string {
	return m.theme.Gradient("visper", true) + m.CursorView()
}

// WordmarkView is the large two-line logo used on the splash screen.
func (m model) WordmarkView() string {
	return m.theme.Gradient(strings.Join(wordmarkLines, "\n"), false)
}
