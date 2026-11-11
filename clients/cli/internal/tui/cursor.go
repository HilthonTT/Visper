package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// cursorState drives every animation in the UI: the blinking logo cursor and
// the spinners. One ticker serves them all; it runs fast only while something
// is actually spinning, so an idle client doesn't redraw several times a second.
type cursorState struct {
	visible bool
	frame   int
}

type CursorTickMsg struct{}

const (
	animFastInterval = 90 * time.Millisecond
	animIdleInterval = 600 * time.Millisecond
	blinkPeriod      = 600 * time.Millisecond
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m model) CursorInit() tea.Cmd {
	return animTick(animFastInterval)
}

func animTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return CursorTickMsg{} })
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

func (m model) CursorUpdate(msg tea.Msg) (model, tea.Cmd) {
	if _, ok := msg.(CursorTickMsg); !ok {
		return m, nil
	}
	m.state.cursor.frame++
	m.state.cursor.visible = time.Now().UnixMilli()/blinkPeriod.Milliseconds()%2 == 0
	if m.animating() {
		return m, animTick(animFastInterval)
	}
	return m, animTick(animIdleInterval)
}

// spinner returns the current spinner glyph in the brand colour.
func (m model) spinner() string {
	f := spinnerFrames[m.state.cursor.frame%len(spinnerFrames)]
	return m.theme.TextBrand().Bold(true).Render(f)
}

func (m model) CursorView() string {
	if m.state.cursor.visible {
		return m.theme.Base().Background(m.theme.Secondary()).Render(" ")
	}
	return m.theme.Base().Render(" ")
}
