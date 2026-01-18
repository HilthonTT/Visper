package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hilthontt/visper/cli/pkg/settings_manager"
	"github.com/hilthontt/visper/cli/pkg/tui/embeds"
)

const (
	waifu1 = 1
	waifu2 = 2
)

type WaifuOption struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type settingsState struct {
	selectedWaifuID  int
	focusedOptionIdx int
}

func LoadWaifus() []WaifuOption {
	data, err := embeds.WaifusData.ReadFile("waifus.json")
	if err != nil {
		log.Fatalf("Failed to read embedded file: %s", err)
	}
	var waifus []WaifuOption
	if err := json.Unmarshal(data, &waifus); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %s", err)
	}
	return waifus
}

func (m model) SettingsSwitch() (model, tea.Cmd) {
	userConfig := m.settingsManager.GetUserConfig()
	m.state.settings.selectedWaifuID = userConfig.SelectedWaifu
	m.state.settings.focusedOptionIdx = userConfig.SelectedWaifu

	m = m.SwitchPage(settingsPage)
	return m, nil
}

func (m model) SettingsUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Back):
			return m.SwitchPage(menuPage), nil

		case msg.Type == tea.KeyRunes:
			if len(msg.String()) == 1 {
				input := msg.String()
				for _, option := range m.waifus {
					if fmt.Sprintf("%d", option.ID) == input {
						m.state.settings.selectedWaifuID = option.ID

						config := &settings_manager.UserConfig{
							SelectedWaifu: option.ID,
						}
						if err := m.settingsManager.SetUserConfig(config); err != nil {
							slog.Error("error saving user config", "error", err)
						}

						return m, nil
					}
				}
			}
		}
	}

	return m, nil
}

func (m model) SettingsView() string {
	var content strings.Builder

	titleStyle := m.theme.Base().
		Bold(true).
		Foreground(m.theme.Brand()).
		MarginBottom(1).
		MarginTop(1)

	content.WriteString(titleStyle.Render("Settings"))
	content.WriteString("\n")

	instructionStyle := m.theme.Base().
		Foreground(m.theme.Body()).
		Italic(true).
		MarginBottom(2)

	content.WriteString(instructionStyle.Render("Press a number to select your chat sidebar image"))

	for _, option := range m.waifus {
		content.WriteString(m.renderWaifuOption(option))
		content.WriteString("\n")
	}

	content.WriteString("\n")
	hintStyle := m.theme.Base().
		Foreground(m.theme.Body()).
		Italic(true).
		MarginTop(1)

	content.WriteString(hintStyle.Render("Press ESC to go back to menu"))

	return content.String()
}

func (m model) renderWaifuOption(option WaifuOption) string {
	isSelected := option.ID == m.state.settings.selectedWaifuID

	var containerStyle lipgloss.Style
	if isSelected {
		containerStyle = m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Highlight()).
			Padding(0, 1).
			MarginBottom(1).
			Width(m.widthContent - 4)
	} else {
		containerStyle = m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Border()).
			Padding(0, 1).
			MarginBottom(1).
			Width(m.widthContent - 4)
	}

	titleStyle := m.theme.Base().Bold(true)
	if isSelected {
		titleStyle = titleStyle.Foreground(m.theme.Highlight())
	}

	descStyle := m.theme.Base().
		Foreground(m.theme.Body()).
		Italic(true)

	var badgeStyle lipgloss.Style
	if isSelected {
		badgeStyle = m.theme.Base().
			Bold(true).
			Foreground(m.theme.Accent()).
			Background(m.theme.Highlight()).
			Padding(0, 1).
			MarginLeft(1)
	} else {
		badgeStyle = m.theme.Base().
			Bold(true).
			Foreground(m.theme.Accent()).
			Padding(0, 0).
			MarginLeft(1)
	}

	var contentBuilder strings.Builder

	titleText := titleStyle.Render(option.Title)
	badgeText := badgeStyle.Render(fmt.Sprintf("%d", option.ID))

	availableWidth := m.widthContent - 8
	titleWidth := lipgloss.Width(titleText)
	badgeWidth := lipgloss.Width(badgeText)
	spacing := max(availableWidth-titleWidth-badgeWidth, 1)

	firstLine := titleText + strings.Repeat(" ", spacing) + badgeText
	contentBuilder.WriteString(firstLine)
	contentBuilder.WriteString("\n")

	contentBuilder.WriteString(descStyle.Render(option.Description))

	if isSelected {
		contentBuilder.WriteString("\n")
		selectedIndicator := m.theme.Base().
			Foreground(m.theme.Highlight()).
			Render("✓ Currently selected")
		contentBuilder.WriteString(selectedIndicator)
	}

	return containerStyle.Render(contentBuilder.String())
}
