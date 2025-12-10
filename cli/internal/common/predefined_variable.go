package common

import "github.com/charmbracelet/lipgloss"

const DeleteMessageTitle = "Are you sure you want to move delete this message?"
const TrashWarnContent = "This operation is irreversible."

const (
	MinimumHeight = 24
	MinimumWidth  = 60

	// TODO : These are model object properties, not global properties
	// We are modifying them in the code many time. They need to be part of model struct.
	MinFooterHeight = 6
	ModalWidth      = 60
	ModalHeight     = 7
)

var (
	LipglossError string
)

func LoadInitialPrerenderedVariables() {
	LipglossError = lipgloss.NewStyle().Foreground(lipgloss.Color("#F93939")).Render("Error") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFEE")).Render(" ┃ ")
}
