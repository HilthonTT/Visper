package tui

import (
	"context"
	"math"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/cli/pkg/tui/theme"
)

type page = int
type size = int

const (
	splashPage page = iota
)

const (
	undersized size = iota
	small
	medium
	large
)

type state struct {
	splash splashState
	cursor cursorState
}

type visibleError struct {
	message string
}

type model struct {
	renderer        *lipgloss.Renderer
	page            page
	state           state
	context         context.Context
	client          *apisdk.Client
	error           *visibleError
	viewportWidth   int
	viewportHeight  int
	widthContainer  int
	heightContainer int
	widthContent    int
	heightContent   int
	size            size
	theme           theme.Theme
}

func NewModel(renderer *lipgloss.Renderer) (tea.Model, error) {
	ctx := context.Background()

	m := model{
		context:  ctx,
		page:     splashPage,
		renderer: renderer,
		state: state{
			splash: splashState{},
		},
		theme: theme.BasicTheme(renderer, nil),
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return m.SplashInit()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewportWidth = msg.Width
		m.viewportHeight = msg.Height

		switch {
		case m.viewportWidth < 20 || m.viewportHeight < 10:
			m.size = undersized
			m.widthContainer = m.viewportWidth
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 50:
			m.size = small
			m.widthContainer = m.viewportWidth
			m.heightContainer = m.viewportHeight
		case m.viewportWidth < 80:
			m.size = medium
			m.widthContainer = 50
			m.heightContainer = int(math.Min(float64(msg.Height), 30))
		default:
			m.size = large
			m.widthContainer = 80
			m.heightContainer = int(math.Min(float64(msg.Height), 30))
		}

		m.widthContent = m.widthContainer - 2
		m.heightContent = m.heightContainer
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.error != nil {
				if m.page == splashPage {
					return m, tea.Quit
				}
				m.error = nil
				return m, nil
			}
		case "ctrl+c":
			return m, tea.Quit
		}
	case CursorTickMsg:
		m, cmd := m.CursorUpdate(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	switch m.page {
	case splashPage:
		m, cmd = m.SplashUpdate(msg)
	}

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	switch m.page {
	case splashPage:
		return m.SplashView()
	}

	return "Test"
}
