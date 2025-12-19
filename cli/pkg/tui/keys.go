package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	// Global navigation
	Quit key.Binding
	Help key.Binding
	Back key.Binding

	// Page navigation
	NewRoomPage  key.Binding
	JoinRoomPage key.Binding
	FaqPage      key.Binding

	// Context-specific (can be enabled/disabled per page)
	Enter  key.Binding
	Tab    key.Binding
	Submit key.Binding
}

var keys = keyMap{
	// Global navigation
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back/cancel"),
	),

	// Page navigation
	NewRoomPage: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new room"),
	),
	JoinRoomPage: key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "join room"),
	),
	FaqPage: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "faq"),
	),

	// Context-specific
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab", "shift+tab"),
		key.WithHelp("tab", "next field"),
	),
	Submit: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "submit"),
	),
}

// FullHelp returns all keybindings for the help view
func (k keyMap) FullHelp() []key.Binding {
	return []key.Binding{
		k.Help,
		k.Quit,
		k.Back,
		k.NewRoomPage,
		k.JoinRoomPage,
		k.FaqPage,
	}
}

// ShortHelp returns minimal keybindings for the compact help view
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}
