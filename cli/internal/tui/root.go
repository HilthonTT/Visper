package tui

import tea "github.com/charmbracelet/bubbletea"

type model struct {
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
