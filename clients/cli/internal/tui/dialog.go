package tui

import (
	"fmt"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/resource"
	"github.com/hilthontt/visper/clients/cli/internal/tui/qr"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
)

// Dialogs are modal overlays; while one is open it takes every key.

type dialogKind int

const (
	// dialogAlert has a single OK button.
	dialogAlert dialogKind = iota
	dialogLeaveRoom
	dialogEditMessage
	dialogDeleteMessage
	dialogNewJoinCode
	dialogKickMember
	dialogQRCode
	dialogRoomInvite
	dialogRoomExpired
	dialogFileExplorer
	// dialogAIConsent asks before the first draft goes to the AI service.
	dialogAIConsent
	// dialogVerifyKey compares a member's key fingerprint and pins it;
	// dialogUnverifyKey is the same for a member already verified.
	dialogVerifyKey
	dialogUnverifyKey
	// dialogRoomFiles lists the room's files; dialogDeleteFile confirms
	// deleting one.
	dialogRoomFiles
	dialogDeleteFile
)

const dialogMaxWidth = 58

type dialogState struct {
	open    bool
	kind    dialogKind
	tone    theme.Tone
	title   string
	content string
	// focus is the highlighted button of a confirm dialog: 0 confirm, 1 cancel.
	focus  int
	qrCode string
	qrSize int
}

// confirmAnswer is what a key press means to a confirm dialog.
type confirmAnswer int

const (
	answerNone confirmAnswer = iota
	answerYes
	answerNo
	answerToggle
)

func (m model) confirmAnswer(msg tea.KeyPressMsg) confirmAnswer {
	switch msg.String() {
	case "y", "Y":
		return answerYes
	case "n", "N", "esc":
		return answerNo
	case "enter", "space":
		if m.state.dialog.focus == 0 {
			return answerYes
		}
		return answerNo
	case "left", "right", "h", "l", "tab", "shift+tab":
		return answerToggle
	}
	return answerNone
}

func (m model) openDialog(d dialogState) model {
	d.open = true
	m.state.dialog = d
	return m
}

func (m model) closeDialog() model {
	m.state.dialog = dialogState{}
	return m
}

// alert opens a dialog with a single OK button.
func (m model) alert(tone theme.Tone, title, content string) model {
	return m.openDialog(dialogState{kind: dialogAlert, tone: tone, title: title, content: content})
}

func (m model) openLeaveRoomDialog() model {
	content := "You'll leave this room. You can come back with the join code while it's still open."
	if m.state.chat.isRoomOwner {
		content = "You own this room, so leaving deletes it for everyone. Messages are gone for good."
	}
	return m.openDialog(dialogState{kind: dialogLeaveRoom, tone: theme.ToneDanger, title: "Leave this room?", content: content})
}

func (m model) openEditMessageDialog(content string) model {
	m.state.chat.editInput.SetValue(content)
	m.state.chat.editInput.CursorEnd()
	m.state.chat.editInput.Focus()
	return m.openDialog(dialogState{kind: dialogEditMessage, title: "Edit message"})
}

func (m model) openDeleteMessageDialog(content string) model {
	if runes := []rune(content); len(runes) > 80 {
		content = string(runes[:80]) + "…"
	}
	return m.openDialog(dialogState{
		kind:    dialogDeleteMessage,
		tone:    theme.ToneDanger,
		title:   "Delete this message?",
		content: fmt.Sprintf("“%s”\n\nIt is removed for everyone in the room.", content),
	})
}

func (m model) openNewJoinCodeDialog() model {
	return m.openDialog(dialogState{
		kind:    dialogNewJoinCode,
		tone:    theme.ToneWarning,
		title:   "Generate a new join code?",
		content: "The current code stops working. People already in the room stay in.",
	})
}

func (m model) openKickMemberDialog(username string) model {
	return m.openDialog(dialogState{
		kind:    dialogKickMember,
		tone:    theme.ToneDanger,
		title:   "Kick " + username + "?",
		content: username + " is removed from the room right away.",
	})
}

func (m model) openQRCodeDialog() model {
	code, size, err := qr.Generate(m.state.chat.room.QRCodeURL)
	if err != nil {
		return m.alert(theme.ToneDanger, "Couldn't make a QR code", err.Error())
	}
	return m.openDialog(dialogState{
		kind:    dialogQRCode,
		title:   "Invite to this room",
		content: m.state.chat.inviteCode(), // shown under the QR code
		qrCode:  code,
		qrSize:  size,
	})
}

// openAIConsentDialog asks the user to agree to sending drafts to the AI
// service, which has to read them: unlike messages, they are not encrypted.
func (m model) openAIConsentDialog() model {
	host := "the Visper server"
	if u, err := url.Parse(resource.Resource.Api.Url); err == nil && u.Host != "" {
		host = u.Host
	}
	return m.openDialog(dialogState{
		kind:  dialogAIConsent,
		tone:  theme.ToneWarning,
		title: "Send your draft unencrypted?",
		content: fmt.Sprintf("AI rewrite sends this draft to the Visper AI service at %s "+
			"WITHOUT end-to-end encryption: whoever runs that server can read it, and it is "+
			"processed and briefly stored there. Your messages stay encrypted.\n\n"+
			"Allow this for the rest of this session?", host),
	})
}

func (m model) openRoomInviteDialog(roomID string) model {
	return m.openDialog(dialogState{
		kind:    dialogRoomInvite,
		title:   "You're invited",
		content: fmt.Sprintf("Someone invited you to room %s. Join now?", roomID),
	})
}

func (m model) openRoomExpiredDialog() model {
	return m.openDialog(dialogState{
		kind:    dialogRoomExpired,
		tone:    theme.ToneWarning,
		title:   "This room has expired",
		content: "Its messages are gone. Taking you back to the start…",
	})
}

// ---------------------------------------------------------------------------
// View

func (m model) dialogWidth() int {
	return max(min(dialogMaxWidth, m.viewportWidth-4), 24)
}

// buttonLabels returns the confirm and cancel labels for a dialog.
func buttonLabels(k dialogKind) (string, string) {
	switch k {
	case dialogLeaveRoom:
		return "Leave", "Stay"
	case dialogDeleteMessage, dialogDeleteFile:
		return "Delete", "Keep"
	case dialogKickMember:
		return "Kick", "Cancel"
	case dialogNewJoinCode:
		return "Generate", "Cancel"
	case dialogRoomInvite:
		return "Join", "Decline"
	case dialogAIConsent:
		return "Send", "Cancel"
	case dialogVerifyKey:
		return "Verify", "Cancel"
	case dialogUnverifyKey:
		return "Unverify", "Close"
	default:
		return "Yes", "No"
	}
}

func (m model) dialogView() string {
	d := m.state.dialog
	if !d.open {
		return ""
	}

	switch d.kind {
	case dialogQRCode:
		return m.qrCodeDialogView()
	case dialogEditMessage:
		return m.editMessageDialogView()
	case dialogFileExplorer:
		return m.fileExplorerView()
	case dialogRoomFiles:
		return m.roomFilesView()
	}

	inner := m.dialogWidth() - 6
	body := m.theme.TextBody().Width(inner).Render(d.content)

	var buttons string
	switch d.kind {
	case dialogAlert, dialogRoomExpired:
		buttons = m.button("OK", "", true, d.tone)
	default:
		yes, no := buttonLabels(d.kind)
		buttons = m.button(no, "", d.focus == 1, theme.ToneInfo) + "  " + m.button(yes, "", d.focus == 0, d.tone)
		hints := m.theme.TextSubtle().Render("y / n · ←→ · enter")
		buttons = spread(inner, hints, buttons)
	}

	return m.dialogFrame(d.tone, d.title, body, buttons)
}

// dialogFrame draws the shared dialog frame: an icon and title, a body and a
// row of buttons, with the border in the tone's colour.
func (m model) dialogFrame(tone theme.Tone, title, body, buttons string) string {
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

func (m model) editMessageDialogView() string {
	inner := m.dialogWidth() - 6
	input := m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(0, 1).
		Width(inner).
		Render(m.state.chat.editInput.View())

	hints := m.keyHints([]keyHint{{"enter", "save"}, {"esc", "cancel"}}, inner)
	return m.dialogFrame(theme.ToneInfo, m.state.dialog.title, input, hints)
}

func (m model) qrCodeDialogView() string {
	d := m.state.dialog
	width := min(max(d.qrSize+8, lipgloss.Width(d.content)+8, 40), max(m.viewportWidth-2, 24))

	pill := m.theme.Pill(m.theme.Brand(), m.theme.OnBrand())
	if lipgloss.Width(d.content)+6 > width {
		pill = pill.Width(width - 6) // wrap rather than cut the code
	}
	code := pill.Render(d.content)
	parts := []string{
		m.theme.TextAccent().Bold(true).Render(d.title),
		m.theme.TextSubtle().Render("scan to join, or share the invite code"),
		"",
		d.qrCode,
		"",
		code,
		"",
		m.theme.TextSubtle().Render("c copy invite code · any other key to close"),
	}

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(1, 2).
		Width(width).
		Align(lipgloss.Center).
		Render(lipgloss.JoinVertical(lipgloss.Center, parts...))
}
