package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/resource"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

func (m model) CreateSDKClient() *apisdk.Client {
	options := []option.RequestOption{
		option.WithBaseURL(resource.Resource.Api.Url),
	}
	return apisdk.NewClient(options...)
}

type splashState struct {
	data  bool
	delay bool
}

type UserSignedInMsg struct {
	client *apisdk.Client
}

type DelayCompleteMsg struct{}

// splashMinDuration keeps the splash on screen long enough to be seen, even
// when the API answers instantly.
const splashMinDuration = 1200 * time.Millisecond

func (m model) healthCmd() tea.Cmd {
	client, ctx := m.client, m.context
	return func() tea.Msg {
		response, err := client.Health.Get(ctx)
		if err != nil {
			return visibleError{message: "Can't reach the Visper server: " + err.Error()}
		}
		return response
	}
}

func (m model) LoadCmds() []tea.Cmd {
	return []tea.Cmd{
		tea.Tick(splashMinDuration, func(time.Time) tea.Msg { return DelayCompleteMsg{} }),
		m.healthCmd(),
	}
}

func (m model) IsLoadingComplete() bool {
	return m.state.splash.data && m.state.splash.delay
}

func (m model) SplashInit() tea.Cmd {
	cmd := func() tea.Msg {
		return UserSignedInMsg{client: m.CreateSDKClient()}
	}

	// Mouse reporting is off by default in Bubble Tea v2 (View.MouseMode).
	return tea.Batch(m.CursorInit(), cmd, tea.RequestBackgroundColor)
}

func (m model) SplashUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case UserSignedInMsg:
		m.client = msg.client

		cmds := m.LoadCmds()
		var notifyCmd tea.Cmd
		m, notifyCmd = m.ensureNotificationWebSocket()
		cmds = append(cmds, notifyCmd)

		return m, tea.Batch(cmds...)

	case DelayCompleteMsg:
		m.state.splash.delay = true
	case *apisdk.HealthResponse:
		m.state.splash.data = true
	case visibleError:
		m.error = &msg
	case tea.KeyPressMsg:
		if m.error != nil && m.client != nil && (msg.String() == "r" || msg.String() == "enter") {
			m.error = nil
			m.state.splash.data = false
			return m, tea.Batch(m.healthCmd(), animTick(animFastInterval))
		}
	}

	if m.IsLoadingComplete() {
		return m.NewRoomSwitch()
	}

	return m, nil
}

func (m model) SplashView() string {
	tagline := m.theme.TextBody().Render("whisper in the terminal")
	features := m.theme.TextSubtle().Render("end-to-end encrypted · anonymous · ephemeral")

	var status string
	if m.error == nil {
		status = m.spinner() + " " + m.theme.TextBody().Render("connecting to "+resource.Resource.Api.Url)
	} else {
		width := min(max(m.viewportWidth-8, 20), 60)
		msg := m.theme.TextError().Width(width - 4).Render(m.error.message)
		box := m.theme.Base().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.theme.Error()).
			Padding(0, 1).
			Render(m.theme.TextError().Bold(true).Render("✕ Connection failed") + "\n" + msg)
		hints := m.keyHints([]footerCommand{{"r", "retry"}, {"esc", "quit"}}, width)
		status = lipgloss.JoinVertical(lipgloss.Center, box, "", hints)
	}

	logo := m.WordmarkView()
	if m.viewportWidth < lipgloss.Width(logo)+4 {
		logo = m.LogoView()
	}

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		logo,
		"",
		tagline,
		features,
		"",
		"",
		status,
	)

	return lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}
