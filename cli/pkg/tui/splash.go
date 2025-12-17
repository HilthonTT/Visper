package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
	"github.com/hilthontt/visper/cli/pkg/resource"
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

func (m model) LoadCmds() []tea.Cmd {
	cmds := []tea.Cmd{}

	cmds = append(cmds, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return DelayCompleteMsg{}
	}))

	cmds = append(cmds, func() tea.Msg {
		response, err := m.client.Health.Get(m.context)
		if err != nil {
			return err
		}
		return response
	})

	return cmds
}

func (m model) IsLoadingComplete() bool {
	return m.state.splash.data && m.state.splash.delay
}

func (m model) SplashInit() tea.Cmd {
	cmd := func() tea.Msg {
		client := m.CreateSDKClient()

		return UserSignedInMsg{
			client: client,
		}
	}

	disableMouseCmd := func() tea.Msg {
		return tea.DisableMouse()
	}

	return tea.Batch(m.CursorInit(), disableMouseCmd, cmd)
}

func (m model) SplashUpdate(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case UserSignedInMsg:
		m.client = msg.client
		return m, tea.Batch(m.LoadCmds()...)
	case DelayCompleteMsg:
		m.state.splash.delay = true
	case apisdk.HealthResponse:
		m.state.splash.data = true
	}

	if m.IsLoadingComplete() {
		// TODO
	}

	return m, nil
}

func (m model) SplashView() string {
	var msg string
	if m.error != nil {
		msg = m.error.message
	} else {
		msg = ""
	}

	var hint string
	if m.error != nil {
		hint = lipgloss.JoinHorizontal(
			lipgloss.Center,
			m.theme.TextAccent().Bold(true).Render("esc"),
			" ",
			"quit",
		)
	} else {
		hint = ""
	}

	if m.error == nil {
		return lipgloss.Place(
			m.viewportWidth,
			m.viewportHeight,
			lipgloss.Center,
			lipgloss.Center,
			m.LogoView(),
		)
	}

	return lipgloss.Place(
		m.viewportWidth,
		m.viewportHeight,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			"",
			"",
			"",
			"",
			m.LogoView(),
			"",
			"",
			lipgloss.JoinHorizontal(
				lipgloss.Center,
				m.theme.TextError().Render(msg),
			),
			hint,
		),
	)
}
