package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	// Global
	Quit     key.Binding
	QuitPage key.Binding // "q" on pages without a text input
	Help     key.Binding
	Menu     key.Binding
	Back     key.Binding
	Enter    key.Binding

	// Page navigation
	NewRoomPage  key.Binding
	JoinRoomPage key.Binding
	FaqPage      key.Binding
	SettingsPage key.Binding
	PrevPage     key.Binding
	NextPage     key.Binding
	PrevTab      key.Binding
	NextTab      key.Binding

	// Chat room
	LeaveRoom       key.Binding
	NewJoinCode     key.Binding
	ToggleSearch    key.Binding
	EditMessages    key.Binding
	SearchMessages  key.Binding
	ShareImage      key.Binding
	AIRewrite       key.Binding
	Invite          key.Binding
	ToggleCompanion key.Binding
	JumpToBottom    key.Binding
}

var keys = keyMap{
	Quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	QuitPage: key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Menu:     key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "menu")),
	Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),

	NewRoomPage:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new room")),
	JoinRoomPage: key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "join room")),
	FaqPage:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "faq")),
	SettingsPage: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "settings")),
	PrevPage:     key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "previous page")),
	NextPage:     key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "next page")),
	PrevTab:      key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous page")),
	NextTab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next page")),

	LeaveRoom:       key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "leave room")),
	NewJoinCode:     key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new join code")),
	ToggleSearch:    key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "search members")),
	EditMessages:    key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "select a message")),
	SearchMessages:  key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "search messages")),
	ShareImage:      key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "share an image")),
	AIRewrite:       key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "rewrite with AI")),
	Invite:          key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "QR invite")),
	ToggleCompanion: key.NewBinding(key.WithKeys("ctrl+w"), key.WithHelp("ctrl+w", "show / hide companion")),
	JumpToBottom:    key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "jump to latest")),
}
