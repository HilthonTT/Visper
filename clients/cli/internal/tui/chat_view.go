package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hilthontt/visper/clients/cli/internal/tui/embeds"
	apisdk "github.com/hilthontt/visper/sdk/go"
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
	chatWaifuWidth     = 36
	chatInputHeight    = 3 // rounded box around the one-line input
	chatStatusHeight   = 1
	chatHeaderHeight   = 2 // bar + rule
	imagePreviewHeight = 15

	// minChatCenterWidth is the narrowest message column the chat is drawn
	// with; below it ChatView shows a "too small" notice instead of rendering
	// with negative widths, which panics inside the viewport/textinput.
	minChatCenterWidth = 30
)

func (m model) chatLeftWidth() int {
	if m.viewportWidth >= 76 {
		return chatMembersWidth
	}
	return 0
}

func (m model) chatRightWidth() int {
	if !m.state.chat.hideWaifu && m.viewportWidth >= 116 {
		return chatWaifuWidth
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

// counterWidth is the space reserved in the input box for the length counter.
const counterWidth = 9

// syncChatLayout sizes the message viewport and inputs for the current window.
// It must run whenever a room is opened, not only on resize: View works on a
// copy of the model, so sizes set there never reach Update, and a freshly
// joined room kept the 50x20 defaults (wrapping and scrolling were computed
// for 20 rows while more were shown, leaving blank rows under the last message).
func (m model) syncChatLayout() model {
	centerWidth := m.chatCenterWidth()

	m.state.chat.messagesViewport.SetWidth(max(centerWidth, 1))
	m.state.chat.messagesViewport.SetHeight(max(m.chatViewportHeight(), 1))
	// box border (2) + padding (2) + prompt (2) + cursor (1) + counter
	m.state.chat.messageInput.SetWidth(max(centerWidth-7-counterWidth, 1))
	m.state.chat.searchInput.SetWidth(max(chatMembersWidth-9, 1))
	m.state.chat.editInput.SetWidth(max(m.dialogWidth()-13, 1))
	return m
}

func (m model) chatLayoutFits() bool {
	return m.chatCenterWidth() >= minChatCenterWidth && m.chatViewportHeight() >= 3
}

// ---------------------------------------------------------------------------
// View

func (m model) ChatView() string {
	if m.size == undersized || !m.chatLayoutFits() {
		return m.chatViewCompact()
	}

	colHeight := m.chatColumnHeight()
	columns := []string{}
	if w := m.chatLeftWidth(); w > 0 {
		columns = append(columns, m.renderParticipantsSidebar(w, colHeight))
	}
	columns = append(columns, m.renderChatCenter(m.chatCenterWidth(), colHeight))
	if w := m.chatRightWidth(); w > 0 {
		columns = append(columns, m.renderRightSidebar(w, colHeight))
	}

	view := lipgloss.JoinVertical(lipgloss.Left,
		m.renderChatHeader(),
		lipgloss.JoinHorizontal(lipgloss.Top, columns...),
	)
	view = m.theme.Base().Width(m.viewportWidth).Height(m.viewportHeight).MaxHeight(m.viewportHeight).Render(view)

	switch {
	case m.state.menu.help:
		return m.overlay(m.HelpView(), view)
	case m.state.notify.open:
		var modal string
		switch m.state.notify.confirmAction {
		case FileExplorerAction:
			modal = m.RenderFileExplorerModal()
		default:
			modal = m.RenderWarnModal()
		}
		return m.overlay(modal, view)
	}
	return view
}

func (m model) renderChatHeader() string {
	width := m.viewportWidth
	c := m.state.chat

	left := []string{m.LogoView()}
	left = append(left, m.theme.Pill(m.theme.Surface(), m.theme.Brand()).Render("# "+c.roomCode))
	if c.room != nil && c.room.EncryptionKey != "" {
		left = append(left, m.theme.TextSuccess().Render("● ")+m.theme.TextBody().Render("encrypted"))
	}

	var right []string
	if c.timeRemaining != "" {
		remaining := time.Until(c.expiresAt)
		style := m.theme.TextBody()
		switch {
		case remaining <= 5*time.Minute:
			style = m.theme.TextError().Bold(true)
		case remaining <= 15*time.Minute:
			style = m.theme.TextWarning().Bold(true)
		}
		right = append(right, m.theme.TextSubtle().Render("expires in ")+style.Render(c.timeRemaining))
	}
	n := len(c.participants)
	people := fmt.Sprintf("%d people", n)
	if n == 1 {
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

func (m model) renderParticipantsSidebar(width, height int) string {
	c := m.state.chat
	inner := width - 3 // right border + 1 cell padding each side

	var lines []string
	title := m.theme.TextSubtle().Bold(true).Render("MEMBERS") + " " +
		m.theme.TextBody().Render(fmt.Sprintf("%d", len(c.participants)))
	lines = append(lines, "", title, "")

	if c.searchActive {
		box := m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Brand()).
			Width(inner).
			Render(c.searchInput.View())
		lines = append(lines, strings.Split(box, "\n")...)
	}

	var ownerID string
	if c.room != nil {
		ownerID = c.room.Owner.ID
	}

	listHeight := height - len(lines) - 2
	rows := 0
	for i, idx := range c.filteredIndices {
		if idx >= len(c.participants) {
			continue
		}
		if rows >= listHeight-1 && len(c.filteredIndices)-i > 1 {
			lines = append(lines, m.theme.TextSubtle().Render(fmt.Sprintf("+%d more", len(c.filteredIndices)-i)))
			break
		}
		p := c.participants[idx]

		var badges string
		if p.ID == ownerID {
			badges += m.theme.TextWarning().Render(" ★")
		}
		if m.userID != nil && p.ID == *m.userID {
			badges += m.theme.TextSubtle().Render(" you")
		}
		nameWidth := inner - 2 - lipgloss.Width(badges)
		name := m.theme.Base().Foreground(m.theme.UserColor(p.Username)).Render(truncate(p.Username, nameWidth))
		dot := m.theme.TextSuccess().Render("●")
		row := dot + " " + name + badges

		if c.searchActive && i == c.memberCursor {
			row = m.theme.Base().Background(m.theme.Surface()).Width(inner).Render(row)
		}
		lines = append(lines, row)
		rows++
	}
	if c.searchActive && len(c.filteredIndices) == 0 {
		lines = append(lines, m.theme.TextSubtle().Render("no one by that name"))
	}

	var hint string
	if c.searchActive {
		if c.isRoomOwner {
			hint = m.keyHints([]footerCommand{{"enter", "kick"}, {"esc", "close"}}, inner)
		} else {
			hint = m.keyHints([]footerCommand{{"esc", "close"}}, inner)
		}
	} else {
		hint = m.keyHints([]footerCommand{{"ctrl+s", "search"}}, inner)
	}

	body := strings.Join(lines, "\n")
	body = lipgloss.PlaceVertical(height-1, lipgloss.Top, body)
	body = lipgloss.JoinVertical(lipgloss.Left, lipgloss.NewStyle().MaxHeight(height-1).Render(body), hint)

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

func (m model) renderChatCenter(width, height int) string {
	c := m.state.chat

	messages := m.theme.Base().
		Width(width).
		Height(m.chatViewportHeight()).
		MaxHeight(m.chatViewportHeight()).
		Render(c.messagesViewport.View())

	// Input box with a length counter on the right.
	inputBorder := m.theme.Border()
	if c.focusedInput == focusMessage && !c.editMode && !m.state.notify.open {
		inputBorder = m.theme.Brand()
	}
	var counter string
	if n, limit := m.messageLength(); n > 0 {
		style := m.theme.TextSubtle()
		switch {
		case n > limit:
			style = m.theme.TextError().Bold(true)
		case n > limit*9/10:
			style = m.theme.TextWarning()
		}
		counter = style.Render(fmt.Sprintf("%d/%d", n, limit))
	}
	inputLine := spread(width-4, c.messageInput.View(), counter)
	input := m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorder).
		Padding(0, 1).
		Width(width).
		Render(ansi.Truncate(inputLine, width-4, ""))

	status := m.theme.Base().PaddingLeft(1).Width(width).MaxWidth(width).Render(m.chatStatusLine(width - 2))

	return lipgloss.JoinVertical(lipgloss.Left, messages, input, status)
}

// messageLength returns the draft's length and the limit it is measured
// against (bytes for encrypted rooms, characters otherwise).
func (m model) messageLength() (int, int) {
	v := m.state.chat.messageInput.Value()
	if m.state.chat.room != nil && m.state.chat.room.EncryptionKey != "" {
		return len(v), maxEncryptedMessageBytes
	}
	return len([]rune(v)), maxMessageLen
}

func (m model) chatStatusLine(width int) string {
	c := m.state.chat
	mode := func(label string) string {
		return m.theme.Pill(m.theme.Secondary(), m.theme.OnBrand()).Render(label) + " "
	}

	switch {
	case c.editMode:
		return mode("EDIT") + m.keyHints([]footerCommand{
			{"↑↓", "select"}, {"enter", "edit"}, {"ctrl+d", "delete"}, {"esc", "done"},
		}, width-7)
	case c.searchActive:
		return mode("SEARCH") + m.keyHints([]footerCommand{{"↑↓", "select"}, {"esc", "close"}}, width-9)
	case c.uploading:
		return m.spinner() + " " + m.theme.TextBrand().Render("Uploading image…")
	case c.aiEnhancing:
		return m.spinner() + " " + m.theme.TextBrand().Render("Rewriting your draft with AI…")
	}

	hints := []footerCommand{
		{"enter", "send"},
		{"ctrl+e", "edit"},
		{"ctrl+u", "image"},
		{"ctrl+a", "AI"},
		{"ctrl+p", "invite"},
		{"ctrl+b", "leave"},
		{"?", "help"},
	}
	if c.unread > 0 {
		unread := m.theme.Pill(m.theme.Brand(), m.theme.OnBrand()).Render(fmt.Sprintf("↓ %d new", c.unread)) + " "
		return unread + m.keyHints(append([]footerCommand{{"end", "jump"}}, hints...), width-lipgloss.Width(unread))
	}
	return m.keyHints(hints, width)
}

// ---------------------------------------------------------------------------
// Messages

// refreshMessages re-renders the message list into the viewport and, in edit
// mode, scrolls so the selected message is visible.
func (m model) refreshMessages() model {
	content, top, bottom := m.renderMessagesWithSelection()
	vp := &m.state.chat.messagesViewport
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

// renderMessagesWithSelection renders the conversation and returns the first
// and last line of the selected message (-1 when nothing is selected).
func (m model) renderMessagesWithSelection() (string, int, int) {
	c := m.state.chat
	width := c.messagesViewport.Width()

	if len(c.messages) == 0 {
		return m.renderEmptyChat(width, c.messagesViewport.Height()), -1, -1
	}

	var userID string
	if m.userID != nil {
		userID = *m.userID
	}

	// 1 cell padding each side, plus the bar and its padding in front of the text.
	contentWidth := max(width-4, 1)

	var lines []string
	selTop, selBottom := -1, -1
	var prev *apisdk.MessageResponse
	for i := range c.messages {
		msg := c.messages[i]
		isOwn := msg.UserID == userID
		selected := c.editMode && isOwn && i == c.selectedMessageIndex

		if prev == nil || dayLabel(prev.CreatedAt) != dayLabel(msg.CreatedAt) {
			label := " " + dayLabel(msg.CreatedAt) + " "
			side := max((width-2-lipgloss.Width(label))/2, 0)
			sepLine := m.rule(side) + m.theme.TextSubtle().Render(label) + m.rule(side)
			if prev != nil {
				lines = append(lines, "")
			}
			lines = append(lines, " "+sepLine, "")
		}

		// Consecutive messages from one person within a few minutes share a header.
		grouped := prev != nil && prev.UserID == msg.UserID &&
			msg.CreatedAt.Sub(prev.CreatedAt) < 5*time.Minute &&
			dayLabel(prev.CreatedAt) == dayLabel(msg.CreatedAt)

		if !grouped {
			if prev != nil {
				lines = append(lines, "")
			}
			name := m.theme.Base().Foreground(m.theme.UserColor(msg.Username)).Bold(true).Render(msg.Username)
			header := " " + name
			if isOwn {
				header += m.theme.TextSubtle().Render(" (you)")
			}
			header += m.theme.TextSubtle().Render("  " + msg.CreatedAt.Format("3:04 PM"))
			if selected {
				header += m.theme.TextSecondary().Bold(true).Render("  ◂ editing")
			}
			lines = append(lines, header)
		} else if selected {
			lines = append(lines, " "+m.theme.TextSecondary().Bold(true).Render("◂ editing"))
		}

		barColor := m.theme.UserColor(msg.Username)
		if c.editMode && !selected {
			barColor = m.theme.Border()
		}
		if selected {
			barColor = m.theme.Secondary()
		}
		body := m.renderMessageContent(msg, isOwn, contentWidth-1)
		block := m.theme.Base().
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(barColor).
			PaddingLeft(1).
			MarginLeft(1).
			Render(body)

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

func (m model) renderEmptyChat(width, height int) string {
	code := m.theme.Pill(m.theme.Surface(), m.theme.Brand()).Render(m.state.chat.roomCode)
	content := lipgloss.JoinVertical(lipgloss.Center,
		m.theme.TextAccent().Bold(true).Render("No messages yet"),
		"",
		m.theme.TextBody().Render("Share the code ")+code+m.theme.TextBody().Render(" to invite people,"),
		m.theme.TextBody().Render("or press ")+m.theme.Key().Render("ctrl+p")+m.theme.TextBody().Render(" for a QR code."),
		"",
		m.theme.TextSubtle().Render("Messages are encrypted before they leave this terminal."),
	)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderMessageContent(msg apisdk.MessageResponse, isOwnMessage bool, width int) string {
	if isImageURL(msg.Content) {
		if m.state.chat.imageFailed[msg.ID] {
			return m.theme.TextSubtle().Italic(true).Render("[image unavailable]")
		}
		if preview, ok := m.state.chat.imagePreviews[msg.ID]; ok {
			return preview
		}
		return m.theme.TextSubtle().Italic(true).Render("loading image…")
	}

	if isOwnMessage {
		return m.theme.TextAccent().Width(width).Render(msg.Content)
	}
	return m.theme.TextAccent().Faint(false).Width(width).Render(msg.Content)
}

// ---------------------------------------------------------------------------
// Companion panel

// waifuDims is the size of the companion image for the current layout.
func (m model) waifuDims() (int, int) {
	return m.chatRightWidth() - 3, m.chatColumnHeight() - 5
}

func (m model) renderRightSidebar(width, height int) string {
	w, h := m.waifuDims()

	// View has a value receiver, so the image must be cached from Update (see
	// withSidebarImage); rendering it here on a miss would decode and resize
	// the PNG on every frame.
	image := m.state.chat.cachedImageContent
	if m.state.chat.cachedImageWidth != w || m.state.chat.cachedImageHeight != h {
		image = m.waifuImage(m.selectedWaifuID(), w, h)
	}
	image = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Bottom, image)

	mood := m.theme.TextSecondary().Italic(true).Width(w).Align(lipgloss.Center).Render(m.state.chat.waifuMood)
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

func (m model) chatViewCompact() string {
	msg := lipgloss.JoinVertical(lipgloss.Center,
		m.LogoView(),
		"",
		m.theme.TextAccent().Render("Make the window a little bigger"),
		m.theme.TextSubtle().Render("to see the conversation"),
	)
	return lipgloss.Place(m.viewportWidth, m.viewportHeight, lipgloss.Center, lipgloss.Center, msg)
}

// ---------------------------------------------------------------------------
// Images

// renderImagePreview renders the fetched image for messageID at the current
// width and caches the result.
func (m model) renderImagePreview(messageID string) model {
	data, ok := m.state.chat.imageFetched[messageID]
	if !ok {
		return m
	}
	width := m.chatCenterWidth() - 6
	if width <= 0 {
		return m
	}
	preview, err := m.imagePreviewer.ImagePreviewFromBytes(data, width, imagePreviewHeight, "")
	if err != nil {
		log.Printf("Failed to render image for message %s: %v", messageID, err)
		m.state.chat.imageFailed[messageID] = true
		return m
	}
	m.state.chat.imagePreviews[messageID] = preview
	return m
}

func (m model) selectedWaifuID() int {
	return m.settingsManager.GetUserConfig().SelectedWaifu
}

// waifuImage renders a companion image at the given size.
func (m model) waifuImage(id, width, height int) string {
	img := embeds.WaifuImage
	if id == waifu2 {
		img = embeds.Waifu2Image
	}
	content, err := m.imagePreviewer.ImagePreviewFromBytes(img, width, height, "")
	if err != nil {
		log.Printf("Failed to load embedded image: %v", err)
		return m.theme.TextSubtle().Render("image unavailable")
	}
	return content
}

// withSidebarImage caches the companion image for the current layout; it must
// be called from Update whenever the layout changes.
func (m model) withSidebarImage() model {
	if m.chatRightWidth() == 0 {
		return m
	}
	w, h := m.waifuDims()
	if w <= 0 || h <= 0 {
		return m
	}
	m.state.chat.cachedImageContent = m.waifuImage(m.selectedWaifuID(), w, h)
	m.state.chat.cachedImageWidth = w
	m.state.chat.cachedImageHeight = h
	return m
}
