package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/qr"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

type ConfirmActionType int

const (
	RenameAction ConfirmActionType = iota
	DeleteAction
	QuitAction
	GoBackAction
	NoAction
	PermanentDeleteAction
	EditMessageAction
	DeleteMessageAction
	NewJoinCodeAction
	KickMemberAction
	ShowQRCodeAction
	RoomInviteAction
	RoomExpiredAction
	FileExplorerAction

	ModalWidth  = 58
	ModalHeight = 9
)

type notifyState struct {
	open          bool
	title         string
	content       string
	confirmAction ConfirmActionType
	tone          theme.Tone
	// focus is the highlighted button of a confirm dialog: 0 confirm, 1 cancel.
	focus  int
	qrCode string
	qrSize int
}

// modalAnswer is what a key press means to a confirm dialog.
type modalAnswer int

const (
	answerNone modalAnswer = iota
	answerYes
	answerNo
	answerToggle
)

func (m model) modalAnswer(msg tea.KeyPressMsg) modalAnswer {
	switch msg.String() {
	case "y", "Y":
		return answerYes
	case "n", "N", "esc":
		return answerNo
	case "enter", "space":
		if m.state.notify.focus == 0 {
			return answerYes
		}
		return answerNo
	case "left", "right", "h", "l", "tab", "shift+tab":
		return answerToggle
	}
	return answerNone
}

// alert opens a dialog with a single OK button.
func (m model) alert(tone theme.Tone, title, content string) model {
	m.state.notify = notifyState{
		open:          true,
		title:         title,
		content:       content,
		confirmAction: NoAction,
		tone:          tone,
	}
	return m
}

func (m model) openWarnModalForLeaveRoom() model {
	content := "You'll leave this room. You can come back with the join code while it's still open."
	if m.state.chat.isRoomOwner {
		content = "You own this room, so leaving deletes it for everyone. Messages are gone for good."
	}
	m.state.notify = notifyState{
		open:          true,
		title:         "Leave this room?",
		content:       content,
		confirmAction: GoBackAction,
		tone:          theme.ToneDanger,
	}
	return m
}

func (m model) closeModal() model {
	m.state.notify = notifyState{open: false}
	return m
}

func (m model) dialogWidth() int {
	return max(min(ModalWidth, m.viewportWidth-4), 24)
}

// buttonLabels returns the confirm and cancel labels for a dialog.
func buttonLabels(a ConfirmActionType) (string, string) {
	switch a {
	case GoBackAction:
		return "Leave", "Stay"
	case DeleteMessageAction:
		return "Delete", "Keep"
	case KickMemberAction:
		return "Kick", "Cancel"
	case NewJoinCodeAction:
		return "Generate", "Cancel"
	case RoomInviteAction:
		return "Join", "Decline"
	default:
		return "Yes", "No"
	}
}

// renderDialog draws the shared dialog frame: an icon and title, a body and a
// row of buttons, with the border in the tone's colour.
func (m model) renderDialog(tone theme.Tone, title, body, buttons string) string {
	width := m.dialogWidth()
	inner := width - 6
	c := m.theme.ToneColor(tone)

	head := m.theme.Base().Foreground(c).Bold(true).Render(theme.ToneIcon(tone)) + "  " +
		m.theme.TextAccent().Bold(true).Render(title)

	parts := []string{head}
	if body != "" {
		parts = append(parts, "", body)
	}
	if buttons != "" {
		parts = append(parts, "", m.theme.Base().Width(inner).Align(lipgloss.Right).Render(buttons))
	}

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c).
		Padding(1, 2).
		Width(width).
		Render(strings.Join(parts, "\n"))
}

func (m model) button(label, hotkey string, focused bool, tone theme.Tone) string {
	text := label
	if hotkey != "" {
		text += " " + hotkey
	}
	if focused {
		return m.theme.Pill(m.theme.ToneColor(tone), m.theme.OnBrand()).Render(text)
	}
	return m.theme.Base().Background(m.theme.Surface()).Foreground(m.theme.Body()).Padding(0, 1).Render(text)
}

func (m model) RenderWarnModal() string {
	n := m.state.notify
	if !n.open {
		return ""
	}

	switch n.confirmAction {
	case ShowQRCodeAction:
		return m.renderQRCodeModal()
	case EditMessageAction:
		return m.RenderEditModal()
	}

	tone := n.tone
	switch n.confirmAction {
	case GoBackAction, DeleteMessageAction, KickMemberAction:
		tone = theme.ToneDanger
	case NewJoinCodeAction, RoomExpiredAction:
		tone = theme.ToneWarning
	}

	inner := m.dialogWidth() - 6
	body := m.theme.TextBody().Width(inner).Render(n.content)

	var buttons string
	switch n.confirmAction {
	case NoAction, RoomExpiredAction:
		buttons = m.button("OK", "", true, tone)
	default:
		yes, no := buttonLabels(n.confirmAction)
		buttons = m.button(no, "", n.focus == 1, theme.ToneInfo) + "  " + m.button(yes, "", n.focus == 0, tone)
		hints := m.theme.TextSubtle().Render("y / n · ←→ · enter")
		buttons = spread(inner, hints, buttons)
	}

	return m.renderDialog(tone, n.title, body, buttons)
}

func (m model) RenderEditModal() string {
	if !m.state.notify.open {
		return ""
	}
	inner := m.dialogWidth() - 6

	input := m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(0, 1).
		Width(inner).
		Render(m.state.chat.editInput.View())

	hints := m.keyHints([]footerCommand{{"enter", "save"}, {"esc", "cancel"}}, inner)
	return m.renderDialog(theme.ToneInfo, m.state.notify.title, input, hints)
}

func (m model) renderQRCodeModal() string {
	n := m.state.notify
	width := max(n.qrSize+8, 40)

	code := m.theme.Pill(m.theme.Brand(), m.theme.OnBrand()).Render(n.content)
	parts := []string{
		m.theme.TextAccent().Bold(true).Render(n.title),
		m.theme.TextSubtle().Render("scan to join, or share the code"),
		"",
		n.qrCode,
		"",
		code,
		"",
		m.theme.TextSubtle().Render("press any key to close"),
	}

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 2).
		Width(width).
		Align(lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, parts...))
}

func (m model) openDeleteMessageModal(messageContent string) model {
	displayContent := messageContent
	if runes := []rune(displayContent); len(runes) > 80 {
		displayContent = string(runes[:80]) + "…"
	}

	m.state.notify = notifyState{
		open:          true,
		title:         "Delete this message?",
		content:       fmt.Sprintf("“%s”\n\nIt is removed for everyone in the room.", displayContent),
		confirmAction: DeleteMessageAction,
	}
	return m
}

func (m model) openNewJoinCodeModal() model {
	m.state.notify = notifyState{
		open:          true,
		title:         "Generate a new join code?",
		content:       "The current code stops working. People already in the room stay in.",
		confirmAction: NewJoinCodeAction,
	}
	return m
}

func (m model) openKickMemberModal(username string) model {
	m.state.notify = notifyState{
		open:          true,
		title:         "Kick " + username + "?",
		content:       username + " is removed from the room right away.",
		confirmAction: KickMemberAction,
	}
	return m
}

func (m model) openQrCodeModal() model {
	qrString, qrSize, err := qr.Generate(m.state.chat.room.QRCodeURL)
	if err != nil {
		return m.alert(theme.ToneDanger, "Couldn't make a QR code", err.Error())
	}

	m.state.notify = notifyState{
		open:          true,
		title:         "Invite to this room",
		content:       m.state.chat.roomCode, // shown under the QR code
		confirmAction: ShowQRCodeAction,
		qrCode:        qrString,
		qrSize:        qrSize,
	}
	return m
}

func (m model) openRoomInviteModal(roomID string) model {
	m.state.notify = notifyState{
		open:          true,
		title:         "You're invited",
		content:       fmt.Sprintf("Someone invited you to room %s. Join now?", roomID),
		confirmAction: RoomInviteAction,
	}
	return m
}
