package tui

import "github.com/charmbracelet/lipgloss"

type ConfirmActionType int

const (
	RenameAction ConfirmActionType = iota
	DeleteAction
	QuitAction
	GoBackAction
	NoAction
	PermanentDeleteAction
	EditMessageAction

	ModalWidth  = 60
	ModalHeight = 9
)

type notifyState struct {
	open          bool
	title         string
	content       string
	confirmAction ConfirmActionType
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
