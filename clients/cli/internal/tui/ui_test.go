package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func press(m model, k string) model {
	if k == "ctrl+e" {
		tm, _ := m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
		return tm.(model)
	}
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "right":
		msg = tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	default:
		r := []rune(k)[0]
		msg = tea.KeyPressMsg{Code: r, Text: k}
	}
	tm, _ := m.Update(msg)
	return tm.(model)
}

func sized(t *testing.T, w, h int) model {
	t.Helper()
	tm, err := NewModel()
	if err != nil {
		t.Fatal(err)
	}
	m := tm.(model)
	m.client = apisdk.NewClient()
	tm, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return tm.(model)
}

func TestTabPagesFitTheWindow(t *testing.T) {
	for _, sz := range [][2]int{{140, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 14}} {
		m := sized(t, sz[0], sz[1])
		m, _ = m.gotoNewRoom()
		for _, p := range []page{newRoomPage, joinRoomPage, faqPage, settingsPage} {
			m, _ = m.gotoPage(p)
			out := m.render()
			if h := lipgloss.Height(out); h > sz[1] {
				t.Errorf("%dx%d page %d: %d lines tall", sz[0], sz[1], p, h)
			}
			for line := range strings.SplitSeq(out, "\n") {
				if w := lipgloss.Width(line); w > sz[0] {
					t.Errorf("%dx%d page %d: line %d cells wide: %q", sz[0], sz[1], p, w, ansi.Strip(line))
					break
				}
			}
		}
	}
}

func TestTabAndArrowsCyclePages(t *testing.T) {
	m := sized(t, 120, 40)
	m, _ = m.gotoNewRoom()
	m = press(m, "tab")
	if m.page != joinRoomPage {
		t.Fatalf("tab went to page %d, want join", m.page)
	}
	// Letters go to the join code field, not the page shortcuts.
	m = press(m, "f")
	if m.page != joinRoomPage || m.state.joinRoom.input.Value() != "f" {
		t.Fatalf("typing on the join page switched pages or was lost (page %d, value %q)", m.page, m.state.joinRoom.input.Value())
	}
	// esc clears the field, a second esc goes back.
	m = press(m, "esc")
	if m.state.joinRoom.input.Value() != "" || m.page != joinRoomPage {
		t.Fatal("first esc should only clear the field")
	}
	m = press(m, "esc")
	if m.page != newRoomPage {
		t.Fatalf("second esc went to page %d, want new room", m.page)
	}
	m = press(m, "f")
	m = press(m, "right")
	if m.page != settingsPage {
		t.Fatalf("right from faq went to page %d, want settings", m.page)
	}
}

func TestMenuPalette(t *testing.T) {
	m := sized(t, 120, 40)
	m, _ = m.gotoNewRoom()
	m = press(m, "m")
	if !m.state.menu.open {
		t.Fatal("m did not open the menu")
	}
	m = press(m, "down")
	m = press(m, "down")
	m = press(m, "enter")
	if m.state.menu.open || m.page != faqPage {
		t.Fatalf("menu enter: open=%v page=%d, want closed on faq", m.state.menu.open, m.page)
	}
	m = press(m, "?")
	if !m.state.menu.help {
		t.Fatal("? did not open help")
	}
	m = press(m, "x")
	if m.state.menu.help || m.page != faqPage {
		t.Fatal("any key should close help without acting")
	}
}

func TestFaqAccordionWraps(t *testing.T) {
	m := sized(t, 120, 40)
	m, _ = m.gotoFaq()
	m = press(m, "up")
	if m.state.faq.cursor != len(m.faqs)-1 {
		t.Fatalf("up from the first question selected %d", m.state.faq.cursor)
	}
	out := ansi.Strip(m.render())
	if !strings.Contains(out, m.faqs[len(m.faqs)-1].Question[:20]) {
		t.Fatal("selected question scrolled out of view")
	}
}

func TestConfirmDialogFocus(t *testing.T) {
	m := newChatModel(t)
	m = m.openNewJoinCodeDialog()
	m.state.chat.isRoomOwner = true

	// ← → move between the buttons; enter on "Cancel" closes without acting.
	m = press(m, "right")
	if m.state.dialog.focus != 1 {
		t.Fatalf("focus = %d after toggle, want 1", m.state.dialog.focus)
	}
	tm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = tm.(model)
	if m.state.dialog.open {
		t.Fatal("enter on cancel left the dialog open")
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(requestFailedMsg); ok {
				t.Fatal("enter on cancel sent the request")
			}
		}
	}
}

func TestChatEventsUseToasts(t *testing.T) {
	m := newChatModel(t)
	m, _ = m.chatUpdate(memberJoinedMsg{member: apisdk.UserResponse{ID: "z", Username: "zesty-lynx"}})
	if m.state.dialog.open {
		t.Fatal("a member joining should not block with a dialog")
	}
	if !strings.Contains(m.state.toast.text, "zesty-lynx") {
		t.Fatalf("toast = %q", m.state.toast.text)
	}
	m, _ = m.chatUpdate(requestFailedMsg{action: "send message", err: errTest})
	if m.state.dialog.open || m.state.toast.tone != theme.ToneDanger {
		t.Fatal("failed request should raise an error toast")
	}
	id := m.state.toast.id
	tm, _ := m.Update(toastExpiredMsg{id: id})
	if tm.(model).state.toast.text != "" {
		t.Fatal("toast did not expire")
	}
}

type testErr struct{}

func (testErr) Error() string { return "boom" }

var errTest = testErr{}

// Selecting a message in edit mode scrolls it into view and redraws.
func TestEditModeScrollsToSelection(t *testing.T) {
	m := newChatModel(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = tm.(model)
	me := m.userID
	now := time.Now()
	for i := range 30 {
		m.state.chat.messages = append(m.state.chat.messages, apisdk.MessageResponse{
			ID: string(rune('a' + i)), UserID: me, Username: "me",
			Content: "message", CreatedAt: now.Add(time.Duration(i) * 10 * time.Minute),
		})
	}
	m = m.refreshMessages()
	m.state.chat.viewport.GotoBottom()

	m = press(m, "ctrl+e")
	if !m.state.chat.edit.active {
		t.Fatal("ctrl+e did not enter edit mode")
	}
	for range 29 {
		m, _ = m.editModeUpdate(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.state.chat.edit.selected != 0 {
		t.Fatalf("selected %d, want 0", m.state.chat.edit.selected)
	}
	if !strings.Contains(m.state.chat.viewport.View(), "selected") {
		t.Fatal("selected message is not visible in the viewport")
	}
}

func TestChatSidePanelsCollapse(t *testing.T) {
	m := newChatModel(t)
	for _, tc := range []struct {
		w           int
		left, right bool
	}{{140, true, true}, {100, true, false}, {70, false, false}} {
		tm, _ := m.Update(tea.WindowSizeMsg{Width: tc.w, Height: 30})
		mm := tm.(model)
		if (mm.chatLeftWidth() > 0) != tc.left || (mm.chatRightWidth() > 0) != tc.right {
			t.Errorf("width %d: left=%d right=%d", tc.w, mm.chatLeftWidth(), mm.chatRightWidth())
		}
	}
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	m = tm.(model)
	m, _ = m.chatUpdate(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if m.chatRightWidth() != 0 {
		t.Fatal("ctrl+w did not hide the companion panel")
	}
	if m.state.chat.viewport.Width() != m.chatCenterWidth() {
		t.Fatal("viewport not resized after hiding the panel")
	}
}
