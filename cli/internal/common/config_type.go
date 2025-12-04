package common

type ThemeType struct {
	// Border
	SidebarBorder string `toml:"sidebar_border"`
	FooterBorder  string `toml:"footer_border"`

	// Border Active
	SidebarBorderActive string `toml:"sidebar_border_active"`
	FooterBorderActive  string `toml:"footer_border_active"`
	ModalBorderActive   string `toml:"modal_border_active"`

	// Background (bg)
	FullScreenBG string `toml:"full_screen_bg"`
	SidebarBG    string `toml:"sidebar_bg"`
	FooterBG     string `toml:"footer_bg"`
	ModalBG      string `toml:"modal_bg"`

	// Special Color
	Cursor        string   `toml:"cursor"`
	Correct       string   `toml:"correct"`
	Error         string   `toml:"error"`
	Hint          string   `toml:"hint"`
	Cancel        string   `toml:"cancel"`
	GradientColor []string `toml:"gradient_color"`

	// Sidebar Special Items
	SidebarTitle          string `toml:"sidebar_title"`
	SidebarItemSelectedFG string `toml:"sidebar_item_selected_fg"`
	SidebarItemSelectedBG string `toml:"sidebar_item_selected_bg"`
	SidebarDivider        string `toml:"sidebar_divider"`

	// Modal Special Items
	ModalCancelFG  string `toml:"modal_cancel_fg"`
	ModalCancelBG  string `toml:"modal_cancel_bg"`
	ModalConfirmFG string `toml:"modal_confirm_fg"`
	ModalConfirmBG string `toml:"modal_confirm_bg"`

	HelpMenuHotkey string `toml:"help_menu_hotkey"`
	HelpMenuTitle  string `toml:"help_menu_title"`
}

type ConfigType struct {
	Theme string `toml:"theme" comment:"More details are at https://superfile.dev/configure/superfile-config/\nchange your theme"`

	SidebarWidth int `toml:"sidebar_width" comment:"\nThe length of the sidebar. If you don't want to display the sidebar, you can input 0 directly. If you want to display the value, please place it in the range of 3-20."`

	BorderTop         string `toml:"border_top" comment:"\nBorder style"`
	BorderBottom      string `toml:"border_bottom"`
	BorderLeft        string `toml:"border_left"`
	BorderRight       string `toml:"border_right"`
	BorderTopLeft     string `toml:"border_top_left"`
	BorderTopRight    string `toml:"border_top_right"`
	BorderBottomLeft  string `toml:"border_bottom_left"`
	BorderBottomRight string `toml:"border_bottom_right"`
	BorderMiddleLeft  string `toml:"border_middle_left"`
	BorderMiddleRight string `toml:"border_middle_right"`

	// IgnoreMissingFields controls whether warnings about missing TOML fields are suppressed.
	IgnoreMissingFields bool `toml:"ignore_missing_fields" comment:"\nWhether to ignore warnings about missing fields in the config file."`
}

// GetIgnoreMissingFields reports whether warnings about missing TOML fields should be ignored.
func (c *ConfigType) GetIgnoreMissingFields() bool {
	return c.IgnoreMissingFields
}

type HotkeysType struct {
	Confirm []string `toml:"confirm" comment:"=================================================================================================\nGlobal hotkeys (cannot conflict with other hotkeys)"`
	Quit    []string `toml:"quit"`
	CdQuit  []string `toml:"cd_quit"`

	// movement
	ListUp   []string `toml:"list_up" comment:"movement"`
	ListDown []string `toml:"list_down"`
	PageUp   []string `toml:"page_up"`
	PageDown []string `toml:"page_down"`

	SearchBar []string `toml:"search_bar"`

	ConfirmTyping []string `toml:"confirm_typing" comment:"=================================================================================================\nTyping hotkeys (can conflict with all hotkeys)"`
	CancelTyping  []string `toml:"cancel_typing"`

	ToggleFooter []string `toml:"toggle_footer"`

	FocusOnSidebar []string `toml:"focus_on_sidebar"`
}
