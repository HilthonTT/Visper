package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/resource"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The splash screen shows while the client checks that the API is reachable.

// splashMinDuration keeps the splash on screen long enough to be seen, even
// when the API answers instantly.
const splashMinDuration = 1200 * time.Millisecond

type splashState struct {
	healthy bool // the API answered the health check
	delayed bool // splashMinDuration has passed
}

type clientReadyMsg struct {
	client *apisdk.Client
}

type splashDelayDoneMsg struct{}

func (m model) splashInit() tea.Cmd {
	// Mouse reporting is off by default in Bubble Tea v2 (View.MouseMode).
	return tea.Batch(
		animTick(animFastInterval),
		func() tea.Msg { return clientReadyMsg{client: newAPIClient()} },
		tea.RequestBackgroundColor,
	)
}

func (m model) healthCmd() tea.Cmd {
	client, ctx := m.client, m.ctx
	return func() tea.Msg {
		response, err := client.Health.Get(ctx)
		if err != nil {
			return visibleError{message: "Can't reach the Visper server: " + err.Error()}
		}
		return response
	}
}

func (m model) splashUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case clientReadyMsg:
		m.client = msg.client
		var notifyCmd tea.Cmd
		m, notifyCmd = m.startNotificationListener()
		return m, tea.Batch(
			tea.Tick(splashMinDuration, func(time.Time) tea.Msg { return splashDelayDoneMsg{} }),
			m.healthCmd(),
			notifyCmd,
		)
	case splashDelayDoneMsg:
		m.state.splash.delayed = true
	case *apisdk.HealthResponse:
		m.state.splash.healthy = true
	case tea.KeyPressMsg:
		if m.error != nil && m.client != nil && (msg.String() == "r" || msg.String() == "enter") {
			m.error = nil
			m.state.splash.healthy = false
			return m, tea.Batch(m.healthCmd(), animTick(animFastInterval))
		}
	}

	if m.state.splash.healthy && m.state.splash.delayed {
		return m.gotoNewRoom()
	}
	return m, nil
}

func (m model) splashView() string {
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
		hints := m.keyHints([]keyHint{{"r", "retry"}, {"esc", "quit"}}, width)
		status = lipgloss.JoinVertical(lipgloss.Center, box, "", hints)
	}

	logo := m.wordmarkView()
	if m.viewportWidth < lipgloss.Width(logo)+4 {
		logo = m.logoView()
	}

	content := lipgloss.JoinVertical(lipgloss.Center, logo, "", tagline, features, "", "", status)
	return lipgloss.Place(m.viewportWidth, m.viewportHeight, lipgloss.Center, lipgloss.Center, content)
}
