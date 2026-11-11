package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	"github.com/hilthontt/visper/clients/cli/internal/tui/embeds"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
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
	selectedWaifuID int
	cursor          int

	// The preview is rendered in Update (View works on a copy of the model,
	// so a cache filled there would be thrown away every frame).
	preview     string
	previewKey  string
	previewSize [2]int
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
	m.state.settings.cursor = 0
	for i, w := range m.waifus {
		if w.ID == userConfig.SelectedWaifu {
			m.state.settings.cursor = i
		}
	}
	m.state.footer.commands = []footerCommand{
		{key: "↑↓", value: "browse"},
		{key: "enter", value: "choose"},
	}

	m = m.SwitchPage(settingsPage)
	return m.withSettingsPreview(), nil
}

func (m model) SettingsUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.withSettingsPreview(), nil
	case tea.KeyPressMsg:
		n := len(m.waifus)
		if n == 0 {
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.state.settings.cursor = (m.state.settings.cursor - 1 + n) % n
			return m.withSettingsPreview(), nil
		case "down":
			m.state.settings.cursor = (m.state.settings.cursor + 1) % n
			return m.withSettingsPreview(), nil
		case "enter", "space":
			return m.chooseWaifu(m.waifus[m.state.settings.cursor])
		}
		for i, option := range m.waifus {
			if msg.String() == fmt.Sprintf("%d", option.ID) {
				m.state.settings.cursor = i
				return m.chooseWaifu(option)
			}
		}
	}
	return m, nil
}

func (m model) chooseWaifu(option WaifuOption) (model, tea.Cmd) {
	m.state.settings.selectedWaifuID = option.ID
	m = m.withSettingsPreview()
	if err := m.settingsManager.SetUserConfig(&settings.UserConfig{SelectedWaifu: option.ID}); err != nil {
		slog.Error("error saving user config", "error", err)
		return m.toast(theme.ToneDanger, "Couldn't save your choice")
	}
	return m.toast(theme.ToneSuccess, option.Title+" will keep you company ♡")
}

// settingsPreviewSize is the size of the companion preview, or 0x0 when the
// terminal is too narrow to show it next to the list.
func (m model) settingsPreviewSize() (int, int) {
	if m.size != large {
		return 0, 0
	}
	return 34, max(min(m.heightContainer-12, 18), 0)
}

func (m model) withSettingsPreview() model {
	w, h := m.settingsPreviewSize()
	if w == 0 || h < 4 || len(m.waifus) == 0 {
		m.state.settings.preview = ""
		return m
	}
	id := m.waifus[m.state.settings.cursor].ID
	k := fmt.Sprintf("%d", id)
	if m.state.settings.previewKey == k && m.state.settings.previewSize == [2]int{w, h} {
		return m
	}
	m.state.settings.preview = m.waifuImage(id, w, h)
	m.state.settings.previewKey = k
	m.state.settings.previewSize = [2]int{w, h}
	return m
}

func (m model) SettingsView(height int) string {
	pw, _ := m.settingsPreviewSize()
	listWidth := min(m.widthContainer-4, 46)
	if pw > 0 {
		listWidth = min(m.widthContainer-pw-8, 42)
	}

	title := m.theme.TextAccent().Bold(true).Render("Chat companion")
	lead := m.theme.TextBody().Width(listWidth).Render("Who keeps you company in the room's side panel.")

	var options []string
	for i, option := range m.waifus {
		options = append(options, m.renderWaifuOption(option, i == m.state.settings.cursor, listWidth))
	}

	left := lipgloss.JoinVertical(lipgloss.Left, title, lead, "", strings.Join(options, "\n"))

	content := left
	if m.state.settings.preview != "" {
		preview := m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Secondary()).
			Render(m.state.settings.preview)
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", preview)
	}

	return m.pageContent(height, content)
}

func (m model) renderWaifuOption(option WaifuOption, focused bool, width int) string {
	isSelected := option.ID == m.state.settings.selectedWaifuID

	radio := m.theme.TextSubtle().Render("○")
	if isSelected {
		radio = m.theme.TextSecondary().Render("●")
	}

	titleStyle := m.theme.TextAccent()
	if focused {
		titleStyle = titleStyle.Bold(true)
	}
	num := m.theme.TextSubtle().Render(fmt.Sprintf("%d", option.ID))
	head := spread(width-4, radio+" "+titleStyle.Render(option.Title), num)
	desc := m.theme.TextBody().PaddingLeft(2).Width(width - 4).Render(option.Description)

	border := m.theme.Border()
	if focused {
		border = m.theme.Brand()
	}
	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width).
		Render(head + "\n" + desc)
}
