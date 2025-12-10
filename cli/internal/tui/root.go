package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/cli/internal/tui/theme"
)

type model struct {
	context         context.Context
	client          *apisdk.Client
	state           state
	error           *visibleError
	theme           theme.Theme
	viewportWidth   int
	viewportHeight  int
	widthContainer  int
	heightContainer int
	widthContent    int
}

type visibleError struct {
	message string
}

type state struct {
	splash SplashState
	cursor cursorState
}

func NewModel() (tea.Model, error) {
	m := model{}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	return "Hello World\n"
}
