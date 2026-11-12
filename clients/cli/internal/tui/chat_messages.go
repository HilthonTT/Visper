package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// groupWindow is how close together consecutive messages from one person
// must be to share a header.
const groupWindow = 5 * time.Minute

// ---------------------------------------------------------------------------
// Finding messages

func (m model) messageContent(messageID string) string {
	for _, msg := range m.state.chat.messages {
		if msg.ID == messageID {
			return msg.Content
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Rendering

// refreshMessages re-renders the message list into the viewport and, in edit
// mode, scrolls so the selected message is visible.
func (m model) refreshMessages() model {
	content, top, bottom := m.renderMessagesWithSelection()
	vp := &m.state.chat.viewport
	vp.SetContent(content)
	if top >= 0 {
		switch {
		case top < vp.YOffset():
			vp.SetYOffset(top)
		case bottom >= vp.YOffset()+vp.Height():
			vp.SetYOffset(bottom - vp.Height() + 1)
		}
	}
	return m
}

func (m model) renderMessages() string {
	s, _, _ := m.renderMessagesWithSelection()
	return s
}

// renderMessagesWithSelection renders the conversation and returns the first
// and last line of the selected message (-1 when nothing is selected).
func (m model) renderMessagesWithSelection() (string, int, int) {
	c := m.state.chat
	width := c.viewport.Width()

	if len(c.messages) == 0 {
		return m.emptyChatView(width, c.viewport.Height()), -1, -1
	}

	// 1 cell padding each side, plus the bar and its padding in front of the text.
	contentWidth := max(width-4, 1)

	var lines []string
	selTop, selBottom := -1, -1
	var prev *apisdk.MessageResponse
	for i := range c.messages {
		msg := c.messages[i]
		isOwn := m.isCurrentUser(msg.UserID)
		var label string
		switch {
		case c.edit.active && i == c.edit.selected:
			label = "◂ selected"
		case i == c.social.search.current():
			label = "◂ " + c.social.search.status()
		}
		selected := label != ""
		newDay := prev == nil || dayLabel(prev.CreatedAt) != dayLabel(msg.CreatedAt)

		if newDay {
			label := " " + dayLabel(msg.CreatedAt) + " "
			side := max((width-2-lipgloss.Width(label))/2, 0)
			sepLine := m.rule(side) + m.theme.TextSubtle().Render(label) + m.rule(side)
			if prev != nil {
				lines = append(lines, "")
			}
			lines = append(lines, " "+sepLine, "")
		}

		// Consecutive messages from one person within a few minutes share a header.
		grouped := !newDay && prev.UserID == msg.UserID && msg.CreatedAt.Sub(prev.CreatedAt) < groupWindow

		switch {
		case !grouped:
			if prev != nil {
				lines = append(lines, "")
			}
			lines = append(lines, m.messageHeader(msg, isOwn, label))
		case selected:
			lines = append(lines, " "+m.theme.TextSecondary().Bold(true).Render(label))
		}

		barColor := m.theme.UserColor(msg.Username)
		switch {
		case selected:
			barColor = m.theme.Secondary()
		case c.edit.active || c.social.search.active:
			barColor = m.theme.Border()
		}
		block := m.theme.Base().
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(barColor).
			PaddingLeft(1).
			MarginLeft(1).
			Render(m.messageBlockView(msg, contentWidth-1))

		blockLines := strings.Split(block, "\n")
		if selected {
			selTop = len(lines) - 1
			selBottom = len(lines) + len(blockLines) - 1
		}
		lines = append(lines, blockLines...)
		prev = &c.messages[i]
	}
	lines = append(lines, "")

	return strings.Join(lines, "\n"), selTop, selBottom
}

// messageHeader is the "name (you)  3:04 PM" line above a group of messages;
// label marks a selected message.
func (m model) messageHeader(msg apisdk.MessageResponse, isOwn bool, label string) string {
	name := m.theme.Base().Foreground(m.theme.UserColor(msg.Username)).Bold(true).Render(msg.Username)
	header := " " + name
	if isOwn {
		header += m.theme.TextSubtle().Render(" (you)")
	}
	header += m.theme.TextSubtle().Render("  " + msg.CreatedAt.Format("3:04 PM"))
	if label != "" {
		header += m.theme.TextSecondary().Bold(true).Render("  " + label)
	}
	return header
}

// messageBlockView is a message's content with the message it replies to
// above it, and its reactions and read receipts below.
func (m model) messageBlockView(msg apisdk.MessageResponse, width int) string {
	var parts []string
	if msg.ReplyToID != "" {
		text := "↳ a message that's no longer here"
		for _, original := range m.state.chat.messages {
			if original.ID == msg.ReplyToID {
				text = "↳ " + quote(original, width-2)
				break
			}
		}
		parts = append(parts, m.theme.TextSubtle().Italic(true).Render(truncate(text, width)))
	}
	parts = append(parts, m.messageContentView(msg, width))
	if line := m.reactionsLine(msg); line != "" {
		parts = append(parts, line)
	}
	if names := m.seenBy(msg); len(names) > 0 {
		parts = append(parts, m.theme.TextSubtle().Render(truncate("✓ seen by "+strings.Join(names, ", "), width)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) messageContentView(msg apisdk.MessageResponse, width int) string {
	if isImageURL(msg.Content) {
		images := m.state.chat.images
		if images.failed[msg.ID] {
			return m.theme.TextSubtle().Italic(true).Render("[image unavailable]")
		}
		if preview, ok := images.previews[msg.ID]; ok {
			return preview
		}
		return m.theme.TextSubtle().Italic(true).Render("loading image…")
	}
	return m.theme.TextAccent().Width(width).Render(msg.Content)
}

func (m model) emptyChatView(width, height int) string {
	content := lipgloss.JoinVertical(lipgloss.Center,
		m.theme.TextAccent().Bold(true).Render("No messages yet"),
		"",
		m.theme.TextBody().Render("Press ")+m.theme.Key().Render("ctrl+p")+m.theme.TextBody().Render(" for the invite code and QR code."),
		m.theme.TextBody().Render("They carry the room's key: share them only with people you invite."),
		"",
		m.theme.TextSubtle().Render("Messages are encrypted before they leave this terminal."),
	)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// dayLabel is the date separator shown between messages of different days.
func dayLabel(t time.Time) string {
	now := time.Now()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today"
	case y1 == y2 && m1 == m2 && d1 == d2-1:
		return "Yesterday"
	default:
		return t.Format("Mon, Jan 2")
	}
}
