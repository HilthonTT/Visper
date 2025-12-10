package common

import "github.com/charmbracelet/lipgloss"

// Generate config error style
func LoadHotkeysError(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("■ ERROR: ") +
		"Hotkeys file \"" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00D9FF")).Render(value) + "\" invalidation"
}
