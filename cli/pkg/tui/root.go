package tui

import (
	"context"
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/cli/pkg/generator"
	"github.com/hilthontt/visper/cli/pkg/tui/theme"
)

type page = int
type size = int

const (
	splashPage page = iota
	faqPage
	newRoomPage
	joinRoomPage
	chatPage
	menuPage
)

const (
	undersized size = iota
	small
	medium
	large
)

type state struct {
	splash   splashState
	cursor   cursorState
	footer   footerState
	menu     menuState
	joinRoom joinRoomState
	newRoom  newRoomState
	chat     chatState
}

type visibleError struct {
	message string
}

type model struct {
	switched        bool
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
	faqs            []FAQ
	generator       *generator.Generator
}

func NewModel(renderer *lipgloss.Renderer, generator *generator.Generator) (tea.Model, error) {
	ctx := context.Background()

	m := model{
		context:  ctx,
		page:     splashPage,
		renderer: renderer,
		state: state{
			splash: splashState{},
			footer: footerState{
				commands: []footerCommand{},
			},
			menu:     menuState{},
			joinRoom: joinRoomState{},
			newRoom:  newRoomState{},
			chat:     chatState{},
		},
		theme:     theme.BasicTheme(renderer, nil),
		faqs:      LoadFaqs(),
		generator: generator,
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return m.SplashInit()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{}

	switch msg := msg.(type) {
	case visibleError:
		m.error = &msg
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
		switch {
		case key.Matches(msg, keys.Back):
			if m.error != nil {
				if m.page == splashPage {
					return m, tea.Quit
				}
				m.error = nil
				return m, nil
			}
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		}
	case CursorTickMsg:
		m, cmd := m.CursorUpdate(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	switch m.page {
	case menuPage:
		m, cmd = m.MenuUpdate(msg)
	case splashPage:
		m, cmd = m.SplashUpdate(msg)
	case joinRoomPage:
		m, cmd = m.JoinRoomUpdate(msg)
	case newRoomPage:
		m, cmd = m.NewRoomUpdate(msg)
	case chatPage:
		m, cmd = m.ChatUpdate(msg)
	}

	var headerCmd tea.Cmd
	m, headerCmd = m.HeaderUpdate(msg)
	cmds = append(cmds, headerCmd)

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if m.switched {
		m.switched = false
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.size == undersized {
		return m.ResizeView()
	}

	switch m.page {
	case splashPage:
		return m.SplashView()
	case chatPage:
		return m.ChatView()
	case menuPage:
		return m.MenuView()
	default:
		header := m.HeaderView()
		footer := m.FooterView()

		// Get content based on current page
		content := m.getContent()

		height := m.heightContainer
		height -= lipgloss.Height(header)
		height -= lipgloss.Height(footer)

		body := m.theme.Base().Width(m.widthContainer).Height(height).Render(content)

		sb := strings.Builder{}
		sb.WriteString(header)
		sb.WriteString(body)
		sb.WriteString(footer)

		child := lipgloss.JoinVertical(
			lipgloss.Left,
			sb.String(),
		)

		return m.renderer.Place(
			m.viewportWidth,
			m.viewportHeight,
			lipgloss.Center,
			lipgloss.Center,
			m.theme.Base().
				MaxWidth(m.widthContainer).
				MaxHeight(m.heightContainer).
				Render(child),
		)
	}
}

func (m model) SwitchPage(page page) model {
	m.page = page
	m.switched = true
	return m
}

func (m model) getContent() string {
	page := "unknown"
	switch m.page {
	case newRoomPage:
		page = m.NewRoomView()
	case joinRoomPage:
		page = m.JoinRoomView()
	case faqPage:
		page = m.FaqView()
	}
	return page
}
