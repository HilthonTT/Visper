package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/hilthontt/visper/cli/pkg/tui/qrfefe"
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

	ModalWidth  = 60
	ModalHeight = 9
)

type notifyState struct {
	open          bool
	title         string
	content       string
	confirmAction ConfirmActionType
	qrCode        string
	qrSize        int
}

func (m model) openWarnModalForLeaveRoom() model {
	m.state.notify = notifyState{
		open:          true,
		title:         "Are you sure you want to leave?",
		content:       "You will leave this room",
		confirmAction: GoBackAction,
	}
	return m
}

func (m model) closeModal() model {
	m.state.notify = notifyState{
		open: false,
	}
	return m
}

func (m model) RenderWarnModal() string {
	if !m.state.notify.open {
		return ""
	}

	if m.state.notify.confirmAction == ShowQRCodeAction {
		return m.renderQRCodeModal()
	}

	var buttons string
	if m.state.notify.confirmAction == NoAction {
		buttons = m.renderOkButton()
	} else {
		buttons = m.renderConfirmCancelButtons()
	}

	// Use a specific width for the inner content
	innerWidth := ModalWidth - 4 // Account for border (2) + padding (2)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent()).
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	contentStyle := lipgloss.NewStyle().
		Foreground(m.theme.Body()).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	title := titleStyle.Render(m.state.notify.title)
	content := contentStyle.Render(m.state.notify.content)

	modalContent := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		content,
		"",
		buttons,
	)

	modalStyle := m.theme.Modal().
		Width(ModalWidth).
		Padding(1, 2)

	return modalStyle.Render(modalContent)
}

func (m model) RenderEditModal() string {
	if !m.state.notify.open {
		return ""
	}

	innerWidth := ModalWidth - 4

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent()).
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	title := titleStyle.Render(m.state.notify.title)

	inputView := m.state.chat.editInput.View()

	hint := lipgloss.NewStyle().
		Foreground(m.theme.Body()).
		Faint(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth).
		Render("Enter to save • Esc to cancel")

	modalContent := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		inputView,
		"",
		hint,
	)

	modalStyle := m.theme.Modal().
		Width(ModalWidth).
		Padding(1, 2)

	return modalStyle.Render(modalContent)
}

func (m model) renderQRCodeModal() string {
	qrModalWidth := max(m.state.notify.qrSize+10, 40)

	innerWidth := qrModalWidth - 4

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent()).
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	title := titleStyle.Render(m.state.notify.title)

	qrCodeStyle := lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	qrCode := qrCodeStyle.Render(m.state.notify.qrCode)

	roomCodeStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent()).
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth)

	roomCodeText := roomCodeStyle.Render(fmt.Sprintf("Room Code: %s", m.state.notify.content))

	hint := lipgloss.NewStyle().
		Foreground(m.theme.Body()).
		Faint(true).
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth).
		Render("Press any key to close")

	modalContent := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		qrCode,
		"",
		roomCodeText,
		"",
		hint,
	)

	modalStyle := m.theme.Modal().
		Width(qrModalWidth).
		Padding(1, 2)

	return modalStyle.Render(modalContent)
}

func (m model) renderOkButton() string {
	innerWidth := ModalWidth - 4

	okButton := lipgloss.NewStyle().
		Foreground(m.theme.Highlight()).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Highlight()).
		Render("OK")

	return lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth).
		Render(okButton)
}

func (m model) renderConfirmCancelButtons() string {
	innerWidth := ModalWidth - 4

	confirmButton := lipgloss.NewStyle().
		Foreground(m.theme.Highlight()).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Highlight()).
		Render("Yes (Y)")

	cancelButton := lipgloss.NewStyle().
		Foreground(m.theme.Body()).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Border()).
		Render("No (N)")

	buttons := lipgloss.JoinHorizontal(
		lipgloss.Center,
		confirmButton,
		"  ",
		cancelButton,
	)

	return lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		Width(innerWidth).
		Render(buttons)
}

func (m model) openDeleteMessageModal(messageContent string) model {
	displayContent := messageContent
	if len(displayContent) > 50 {
		displayContent = displayContent[:50] + "..."
	}

	m.state.notify = notifyState{
		open:          true,
		title:         "Delete Message",
		content:       fmt.Sprintf("Are you sure you want to delete this message?\n\n\"%s\"", displayContent),
		confirmAction: DeleteMessageAction,
	}
	return m
}

func (m model) openNewJoinCodeModal() model {
	m.state.notify = notifyState{
		open:          true,
		title:         "Generate New Join Code",
		content:       "Are you sure you want to generate a new join code?\nThe old code will no longer work.",
		confirmAction: NewJoinCodeAction,
	}

	return m
}

func (m model) openKickMemberModal(username string) model {
	m.state.notify = notifyState{
		open:          true,
		title:         "Kick Member",
		content:       fmt.Sprintf("Are you sure you want to kick %s from the room?", username),
		confirmAction: KickMemberAction,
	}

	return m
}

func (m model) openQrCodeModal() model {
	qrString, qrSize, err := qrfefe.Generate(10, m.state.chat.room.QRCodeURL)
	if err != nil {
		m.state.notify = notifyState{
			open:          true,
			title:         "Error",
			content:       fmt.Sprintf("Failed to generate QR code: %v", err),
			confirmAction: NoAction,
		}
		return m
	}

	m.state.notify = notifyState{
		open:          true,
		title:         "Room QR Code",
		content:       m.state.chat.roomCode, // Store room code for display
		confirmAction: ShowQRCodeAction,
		qrCode:        qrString,
		qrSize:        qrSize,
	}

	return m
}
