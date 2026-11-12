package tui

import (
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/settings"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The settings page picks the chat companion and shows this terminal's
// device code, which the web client needs to send it invites.

type settingsState struct {
	selectedID int
	cursor     int

	// The preview is rendered in Update (View works on a copy of the model,
	// so a cache filled there would be thrown away every frame).
	preview     string
	previewID   int
	previewSize [2]int
}

func (m model) gotoSettings() (model, tea.Cmd) {
	selected := m.selectedCompanionID()
	m.state.settings.selectedID = selected
	m.state.settings.cursor = 0
	for i, c := range m.companions {
		if c.ID == selected {
			m.state.settings.cursor = i
		}
	}
	m = m.setFooterHints(keyHint{"↑↓", "browse"}, keyHint{"enter", "choose"}, keyHint{"c", "copy device code"})
	m = m.switchPage(settingsPage)
	return m.withSettingsPreview(), nil
}

func (m model) settingsUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.withSettingsPreview(), nil
	case tea.KeyPressMsg:
		if msg.String() == "c" {
			return m.copyDeviceCode()
		}
		n := len(m.companions)
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
			return m.chooseCompanion(m.companions[m.state.settings.cursor])
		}
		for i, c := range m.companions {
			if msg.String() == strconv.Itoa(c.ID) {
				m.state.settings.cursor = i
				return m.chooseCompanion(c)
			}
		}
	}
	return m, nil
}

func (m model) chooseCompanion(c companion) (model, tea.Cmd) {
	m.state.settings.selectedID = c.ID
	m = m.withSettingsPreview()
	if err := m.settingsManager.SetUserConfig(&settings.UserConfig{SelectedWaifu: c.ID}); err != nil {
		slog.Error("error saving user config", "error", err)
		return m.toast(theme.ToneDanger, "Couldn't save your choice")
	}
	return m.toast(theme.ToneSuccess, c.Title+" will keep you company ♡")
}

// deviceCode is what the web client's Join page asks for: this terminal's
// user ID and the public key the room key is sealed to. Both change on every
// start (and the user ID may change when a room is joined).
func (m model) deviceCode() string {
	if m.deviceKey == nil || m.userID == "" {
		return ""
	}
	return apisdk.FormatDeviceCode(m.userID, m.deviceKey)
}

func (m model) copyDeviceCode() (model, tea.Cmd) {
	code := m.deviceCode()
	if code == "" {
		return m, nil
	}
	m, cmd := m.toast(theme.ToneSuccess, "Device code copied. Paste it into the web Join page.")
	return m, tea.Batch(tea.SetClipboard(code), cmd)
}

// settingsPreviewSize is the size of the companion preview, or 0x0 when the
// terminal is too narrow to show it next to the list.
func (m model) settingsPreviewSize() (int, int) {
	if m.size != large {
		return 0, 0
	}
	return 34, max(min(m.heightContainer-12, 18), 0)
}

// withSettingsPreview renders the highlighted companion's preview, unless the
// cached one is still current.
func (m model) withSettingsPreview() model {
	s := &m.state.settings
	w, h := m.settingsPreviewSize()
	if w == 0 || h < 4 || len(m.companions) == 0 {
		s.preview = ""
		return m
	}
	id := m.companions[s.cursor].ID
	if s.preview != "" && s.previewID == id && s.previewSize == [2]int{w, h} {
		return m
	}
	s.preview = m.companionImage(id, w, h)
	s.previewID = id
	s.previewSize = [2]int{w, h}
	return m
}

func (m model) settingsView(height int) string {
	pw, _ := m.settingsPreviewSize()
	listWidth := min(m.widthContainer-4, 46)
	if pw > 0 {
		listWidth = min(m.widthContainer-pw-8, 42)
	}

	title := m.theme.TextAccent().Bold(true).Render("Chat companion")
	lead := m.theme.TextBody().Width(listWidth).Render("Who keeps you company in the room's side panel.")

	options := make([]string, 0, len(m.companions))
	for i, c := range m.companions {
		options = append(options, m.companionOptionView(c, i == m.state.settings.cursor, listWidth))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, lead, "", strings.Join(options, "\n"))
	if code := m.deviceCode(); code != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "",
			m.theme.TextAccent().Bold(true).Render("Device code"),
			m.theme.TextBody().Width(listWidth).Render("Paste it into the web client's Join page to send invites here. The room key is sealed to this terminal, so the server can't read it."),
			m.theme.TextSecondary().Width(listWidth).Render(code),
			m.theme.TextSubtle().Render("c copy · changes every time Visper starts"),
		)
	}
	if m.state.settings.preview != "" {
		preview := m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Secondary()).
			Render(m.state.settings.preview)
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, "    ", preview)
	}

	return m.pageContent(height, content)
}

func (m model) companionOptionView(c companion, focused bool, width int) string {
	radio := m.theme.TextSubtle().Render("○")
	if c.ID == m.state.settings.selectedID {
		radio = m.theme.TextSecondary().Render("●")
	}

	titleStyle := m.theme.TextAccent()
	border := m.theme.Border()
	if focused {
		titleStyle = titleStyle.Bold(true)
		border = m.theme.Brand()
	}
	num := m.theme.TextSubtle().Render(strconv.Itoa(c.ID))
	head := spread(width-4, radio+" "+titleStyle.Render(c.Title), num)
	desc := m.theme.TextBody().PaddingLeft(2).Width(width - 4).Render(c.Description)

	return m.theme.Base().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width).
		Render(head + "\n" + desc)
}
