package strfn

import (
	"strings"
	"unicode/utf8"
)

// Text from the network (message text, usernames, file names, server error
// messages) is written straight to the terminal. Control characters in it
// would let another member or the server send escape sequences: recolour or
// overwrite the screen, retitle the window, write to the clipboard (OSC 52)
// or plant hyperlinks (OSC 8).

// CleanText removes control characters (C0 except newline and tab, DEL and
// C1) from untrusted text, so it can't carry terminal escape sequences.
// Invalid UTF-8 becomes U+FFFD.
func CleanText(s string) string {
	if !hasControl(s, true) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

// CleanLine is CleanText for single-line text (names, codes): newlines and
// tabs become spaces too.
func CleanLine(s string) string {
	if !hasControl(s, false) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

// hasControl reports whether s has control characters to remove, or
// invalid UTF-8 (a lone 0x9b byte is CSI to some terminals), which
// strings.Map replaces.
func hasControl(s string, keepNewlines bool) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if isControl(r) && !(keepNewlines && (r == '\n' || r == '\t')) {
			return true
		}
	}
	return false
}
