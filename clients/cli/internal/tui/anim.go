package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// animState drives every animation in the UI: the blinking logo cursor and
// the spinners. One ticker serves them all; it runs fast only while something
// is actually spinning, so an idle client doesn't redraw several times a second.
type animState struct {
	cursorVisible bool
	frame         int
}

type animTickMsg struct{}

const (
	animFastInterval = 90 * time.Millisecond
	animIdleInterval = 600 * time.Millisecond
	blinkPeriod      = 600 * time.Millisecond
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// wordmarkLines spells "visper" in half-block letters.
var wordmarkLines = []string{
	"█ █ █ █▀▀ █▀█ █▀▀ █▀█",
	"▀▄▀ █ ▄▄█ █▀▀ ██▄ █▀▄",
}

// animTick schedules the next animation frame. Call it with animFastInterval
// when starting something that shows a spinner, so it spins right away.
func animTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return animTickMsg{} })
}

// animating reports whether a spinner is on screen.
func (m model) animating() bool {
	switch {
	case m.page == splashPage && m.error == nil:
		return true
	case m.state.newRoom.creating, m.state.joinRoom.joining:
		return true
	case m.state.chat.aiEnhancing, m.state.chat.uploading:
		return true
	}
	return false
}

func (m model) animUpdate() (model, tea.Cmd) {
	m.state.anim.frame++
	m.state.anim.cursorVisible = time.Now().UnixMilli()/blinkPeriod.Milliseconds()%2 == 0
	if m.animating() {
		return m, animTick(animFastInterval)
	}
	return m, animTick(animIdleInterval)
}

// spinner returns the current spinner glyph in the brand colour.
func (m model) spinner() string {
	f := spinnerFrames[m.state.anim.frame%len(spinnerFrames)]
	return m.theme.TextBrand().Bold(true).Render(f)
}

func (m model) cursorView() string {
	if m.state.anim.cursorVisible {
		return m.theme.Base().Background(m.theme.Secondary()).Render(" ")
	}
	return m.theme.Base().Render(" ")
}

// logoView is the compact, single-line logo: the name and a blinking cursor.
func (m model) logoView() string {
	return m.theme.Gradient("visper", true) + m.cursorView()
}

// wordmarkView is the large two-line logo used on the splash screen.
func (m model) wordmarkView() string {
	return m.theme.Gradient(strings.Join(wordmarkLines, "\n"), false)
}
