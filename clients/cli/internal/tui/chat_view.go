package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------------
// Layout
//
//	┌ top bar ──────────────────────────────────────────────────────────┐
//	│ members │ messages                                   │ companion │
//	│         │ ╭ input ─────────────────────────────────╮ │           │
//	│         │ ╰────────────────────────────────────────╯ │           │
//	│         │ status line                                │           │
//
// The side panels drop away on narrow terminals, members last.

const (
	chatMembersWidth   = 24
	chatCompanionWidth = 36
	chatInputHeight    = 3 // rounded box around the one-line input
	chatStatusHeight   = 1
	chatHeaderHeight   = 2 // bar + rule

	// Window widths from which the side panels are shown.
	chatMembersMinWindow   = 76
	chatCompanionMinWindow = 116

	// minChatCenterWidth is the narrowest message column the chat is drawn
	// with; below it chatView shows a "too small" notice instead of rendering
	// with negative widths, which panics inside the viewport/textinput.
	minChatCenterWidth = 30

	// counterWidth is the space reserved in the input box for the length counter.
	counterWidth = 9
)

func (m model) chatLeftWidth() int {
	if m.viewportWidth >= chatMembersMinWindow {
		return chatMembersWidth
	}
	return 0
}

func (m model) chatRightWidth() int {
	if !m.state.chat.companion.hidden && m.viewportWidth >= chatCompanionMinWindow {
		return chatCompanionWidth
	}
	return 0
}

func (m model) chatCenterWidth() int {
	return m.viewportWidth - m.chatLeftWidth() - m.chatRightWidth()
}

func (m model) chatColumnHeight() int {
	return m.viewportHeight - chatHeaderHeight
}

func (m model) chatViewportHeight() int {
	return m.chatColumnHeight() - chatInputHeight - chatStatusHeight
}

func (m model) chatLayoutFits() bool {
	return m.chatCenterWidth() >= minChatCenterWidth && m.chatViewportHeight() >= 3
}

// syncChatLayout sizes the message viewport and inputs for the current window.
// It must run whenever a room is opened, not only on resize: View works on a
// copy of the model, so sizes set there never reach Update, and a freshly
// joined room kept the 50x20 defaults (wrapping and scrolling were computed
// for 20 rows while more were shown, leaving blank rows under the last message).
func (m model) syncChatLayout() model {
	c := &m.state.chat
	centerWidth := m.chatCenterWidth()

	c.viewport.SetWidth(max(centerWidth, 1))
	c.viewport.SetHeight(max(m.chatViewportHeight(), 1))
	// box border (2) + padding (2) + prompt (2) + cursor (1) + counter
	c.messageInput.SetWidth(max(centerWidth-7-counterWidth, 1))
	c.members.search.SetWidth(max(chatMembersWidth-9, 1))
	c.editInput.SetWidth(max(m.dialogWidth()-13, 1))
	return m
}

// relayoutChat resizes everything that depends on the window size.
func (m model) relayoutChat() model {
	m = m.syncChatLayout()
	m = m.rerenderImagePreviews()
	m = m.withCompanionImage()
	return m.refreshMessages()
}

// ---------------------------------------------------------------------------
// View

func (m model) chatView() string {
	if !m.chatLayoutFits() {
		return m.chatTooSmallView()
	}

	colHeight := m.chatColumnHeight()
	var columns []string
	if w := m.chatLeftWidth(); w > 0 {
		columns = append(columns, m.membersPanelView(w, colHeight))
	}
	columns = append(columns, m.chatCenterView(m.chatCenterWidth()))
	if w := m.chatRightWidth(); w > 0 {
		columns = append(columns, m.companionPanelView(w, colHeight))
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		m.chatTopBarView(),
		lipgloss.JoinHorizontal(lipgloss.Top, columns...),
	)
	view = m.theme.Base().Width(m.viewportWidth).Height(m.viewportHeight).MaxHeight(m.viewportHeight).Render(view)

	switch {
	case m.state.menu.help:
		return m.overlay(m.helpView(), view)
	case m.state.dialog.open:
		return m.overlay(m.dialogView(), view)
	}
	return view
}

func (m model) chatTopBarView() string {
	width := m.viewportWidth
	c := m.state.chat

	left := []string{
		m.logoView(),
		m.theme.Pill(m.theme.Surface(), m.theme.Brand()).Render("# " + c.roomCode),
	}
	if c.encrypted() {
		left = append(left, m.theme.TextSuccess().Render("● ")+m.theme.TextBody().Render("encrypted"))
	}

	var right []string
	if c.expiry.remaining != "" {
		remaining := time.Until(c.expiry.expiresAt)
		style := m.theme.TextBody()
		switch {
		case remaining <= expiryWarning:
			style = m.theme.TextError().Bold(true)
		case remaining <= 15*time.Minute:
			style = m.theme.TextWarning().Bold(true)
		}
		right = append(right, m.theme.TextSubtle().Render("expires in ")+style.Render(c.expiry.remaining))
	}
	people := fmt.Sprintf("%d people", len(c.members.list))
	if len(c.members.list) == 1 {
		people = "just you"
	}
	right = append(right, m.theme.TextBody().Render(people))
	if c.isRoomOwner {
		right = append(right, m.theme.TextWarning().Render("★ ")+m.theme.TextBody().Render("owner"))
	}

	sep := m.theme.TextSubtle().Render("  │  ")
	l := " " + strings.Join(left, "  ")
	r := strings.Join(right, sep) + " "
	// Drop right-hand items until the bar fits.
	for len(right) > 0 && lipgloss.Width(l)+lipgloss.Width(r)+2 > width {
		right = right[1:]
		r = strings.Join(right, sep) + " "
	}

	return lipgloss.JoinVertical(lipgloss.Left, spread(width, l, r), m.rule(width))
}

func (m model) membersPanelView(width, height int) string {
	c := m.state.chat
	s := c.members
	inner := width - 3 // right border + 1 cell padding each side

	title := m.theme.TextSubtle().Bold(true).Render("MEMBERS") + " " +
		m.theme.TextBody().Render(strconv.Itoa(len(s.list)))
	lines := []string{"", title, ""}

	if s.searching {
		box := m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Brand()).
			Width(inner).
			Render(s.search.View())
		lines = append(lines, strings.Split(box, "\n")...)
	}

	var ownerID string
	if c.room != nil {
		ownerID = c.room.Owner.ID
	}

	listHeight := height - len(lines) - 2
	rows := 0
	for i, idx := range s.filtered {
		if idx >= len(s.list) {
			continue
		}
		if rows >= listHeight-1 && len(s.filtered)-i > 1 {
			lines = append(lines, m.theme.TextSubtle().Render(fmt.Sprintf("+%d more", len(s.filtered)-i)))
			break
		}
		p := s.list[idx]

		var badges string
		if p.ID == ownerID {
			badges += m.theme.TextWarning().Render(" ★")
		}
		if m.isCurrentUser(p.ID) {
			badges += m.theme.TextSubtle().Render(" you")
		}
		nameWidth := inner - 2 - lipgloss.Width(badges)
		name := m.theme.Base().Foreground(m.theme.UserColor(p.Username)).Render(truncate(p.Username, nameWidth))
		dot := m.theme.TextSuccess().Render("●")
		if c.social.online != nil && !c.social.online[p.ID] {
			dot = m.theme.TextSubtle().Render("○") // known to be offline
		}
		row := dot + " " + name + badges

		if s.searching && i == s.cursor {
			row = m.theme.Base().Background(m.theme.Surface()).Width(inner).Render(row)
		}
		lines = append(lines, row)
		rows++
	}
	if s.searching && len(s.filtered) == 0 {
		lines = append(lines, m.theme.TextSubtle().Render("no one by that name"))
	}

	var hints []keyHint
	switch {
	case s.searching && c.isRoomOwner:
		hints = []keyHint{{"enter", "kick"}, {"esc", "close"}}
	case s.searching:
		hints = []keyHint{{"esc", "close"}}
	default:
		hints = []keyHint{{"ctrl+s", "search"}}
	}

	body := lipgloss.PlaceVertical(height-1, lipgloss.Top, strings.Join(lines, "\n"))
	body = lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().MaxHeight(height-1).Render(body),
		m.keyHints(hints, inner))

	return m.theme.Base().
		Width(width).
		Height(height).
		MaxHeight(height).
		PaddingLeft(1).
		PaddingRight(1).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Render(body)
}

// chatCenterView is the message list, the input box and the status line.
func (m model) chatCenterView(width int) string {
	c := m.state.chat

	messages := m.theme.Base().
		Width(width).
		Height(m.chatViewportHeight()).
		MaxHeight(m.chatViewportHeight()).
		Render(c.viewport.View())

	// Input box with a length counter on the right.
	inputBorder := m.theme.Border()
	if !c.members.searching && !c.edit.active && !m.state.dialog.open {
		inputBorder = m.theme.Brand()
	}
	inputView := c.messageInput.View()
	var counter string
	if c.social.search.active {
		inputView = c.social.search.input.View()
	} else if n, limit := m.messageLength(); n > 0 {
		style := m.theme.TextSubtle()
		switch {
		case n > limit:
			style = m.theme.TextError().Bold(true)
		case n > limit*9/10:
			style = m.theme.TextWarning()
		}
		counter = style.Render(fmt.Sprintf("%d/%d", n, limit))
	}
	inputLine := spread(width-4, inputView, counter)
	input := m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorder).
		Padding(0, 1).
		Width(width).
		Render(ansi.Truncate(inputLine, width-4, ""))

	status := m.theme.Base().PaddingLeft(1).Width(width).MaxWidth(width).Render(m.chatStatusLine(width - 2))

	return lipgloss.JoinVertical(lipgloss.Left, messages, input, status)
}

// chatHints are the status line hints when nothing else is going on.
var chatHints = []keyHint{
	{"enter", "send"},
	{"ctrl+e", "select"},
	{"ctrl+f", "find"},
	{"ctrl+u", "image"},
	{"ctrl+a", "AI"},
	{"ctrl+p", "invite"},
	{"ctrl+b", "leave"},
	{"?", "help"},
}

func (m model) chatStatusLine(width int) string {
	c := m.state.chat
	mode := func(label string) string {
		return m.theme.Pill(m.theme.Secondary(), m.theme.OnBrand()).Render(label) + " "
	}

	switch {
	case c.edit.active:
		return mode("SELECT") + m.keyHints([]keyHint{
			{"↑↓", "move"}, {"r", "reply"}, {"1-6", "react"}, {"enter", "edit"}, {"ctrl+d", "delete"}, {"esc", "done"},
		}, width-9)
	case c.social.search.active:
		found := m.theme.TextBody().Render(c.social.search.status()) + " "
		return mode("FIND") + found + m.keyHints([]keyHint{{"↑↓", "older/newer"}, {"esc", "close"}}, width-7-lipgloss.Width(found))
	case c.members.searching:
		return mode("SEARCH") + m.keyHints([]keyHint{{"↑↓", "select"}, {"esc", "close"}}, width-9)
	case c.uploading:
		return m.spinner() + " " + m.theme.TextBrand().Render("Uploading image…")
	case c.aiEnhancing:
		return m.spinner() + " " + m.theme.TextBrand().Render("Rewriting your draft with AI…")
	case c.social.replyTo != "":
		label := mode("REPLY")
		hint := m.keyHints([]keyHint{{"esc", "cancel"}}, 14)
		target := "a message that's no longer here"
		if original, ok := m.replyTarget(); ok {
			target = quote(original, width)
		}
		room := max(width-lipgloss.Width(label)-lipgloss.Width(hint)-3, 4)
		return label + m.theme.TextBody().Render("↳ "+truncate(target, room-2)) + " " + hint
	case c.unread > 0:
		unread := m.theme.Pill(m.theme.Brand(), m.theme.OnBrand()).Render(fmt.Sprintf("↓ %d new", c.unread)) + " "
		return unread + m.keyHints(append([]keyHint{{"end", "jump"}}, chatHints...), width-lipgloss.Width(unread))
	}
	if typing := c.social.typingText(); typing != "" {
		t := m.theme.TextBrand().Italic(true).Render(typing) + "  "
		return t + m.keyHints(chatHints, max(width-lipgloss.Width(t), 0))
	}
	return m.keyHints(chatHints, width)
}

// ---------------------------------------------------------------------------
// Companion panel

// companionImageSize is the size of the companion image for the current layout.
func (m model) companionImageSize() (int, int) {
	return m.chatRightWidth() - 3, m.chatColumnHeight() - 5
}

// withCompanionImage caches the companion image for the current layout; it
// must be called from Update whenever the layout changes.
func (m model) withCompanionImage() model {
	if m.chatRightWidth() == 0 {
		return m
	}
	w, h := m.companionImageSize()
	if w <= 0 || h <= 0 {
		return m
	}
	p := &m.state.chat.companion
	p.image = m.companionImage(m.selectedCompanionID(), w, h)
	p.imageWidth, p.imageHeight = w, h
	return m
}

func (m model) companionPanelView(width, height int) string {
	w, h := m.companionImageSize()
	p := m.state.chat.companion

	// View has a value receiver, so the image must be cached from Update (see
	// withCompanionImage); rendering it here on a miss would decode and resize
	// the PNG on every frame.
	image := p.image
	if p.imageWidth != w || p.imageHeight != h {
		image = m.companionImage(m.selectedCompanionID(), w, h)
	}
	image = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Bottom, image)

	mood := m.theme.TextSecondary().Italic(true).Width(w).Align(lipgloss.Center).Render(p.mood)
	hint := m.theme.TextSubtle().Width(w).Align(lipgloss.Center).Render("ctrl+w hide")

	return m.theme.Base().
		Width(width).
		Height(height).
		MaxHeight(height).
		PaddingLeft(1).
		PaddingRight(1).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border()).
		Render(lipgloss.JoinVertical(lipgloss.Left, "", image, "", mood, hint))
}

func (m model) chatTooSmallView() string {
	msg := lipgloss.JoinVertical(lipgloss.Center,
		m.logoView(),
		"",
		m.theme.TextAccent().Render("Make the window a little bigger"),
		m.theme.TextSubtle().Render("to see the conversation"),
	)
	return lipgloss.Place(m.viewportWidth, m.viewportHeight, lipgloss.Center, lipgloss.Center, msg)
}
