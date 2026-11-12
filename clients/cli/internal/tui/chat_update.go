package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

func (m model) chatUpdate(msg tea.Msg) (model, tea.Cmd) {
	if mm, cmd, handled := m.chatEventUpdate(msg); handled {
		return mm, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.relayoutChat()
	case tea.KeyPressMsg:
		if mm, cmd, handled := m.chatKeyUpdate(msg); handled {
			return mm, cmd
		}
	}

	if m.state.dialog.open {
		return m, nil
	}
	return m.updateChatInputs(msg)
}

// chatKeyUpdate handles the chat's own keys; handled is false for keys that
// go on to the focused input and the message list.
func (m model) chatKeyUpdate(msg tea.KeyPressMsg) (_ model, _ tea.Cmd, handled bool) {
	c := m.state.chat
	switch {
	case m.state.dialog.open:
		mm, cmd := m.chatDialogUpdate(msg)
		return mm, cmd, true
	case c.edit.active:
		mm, cmd := m.editModeUpdate(msg)
		return mm, cmd, true
	case c.social.search.active:
		mm, cmd := m.messageSearchKeyUpdate(msg)
		return mm, cmd, true
	case c.members.searching:
		if mm, cmd, handled := m.memberSearchKeyUpdate(msg); handled {
			return mm, cmd, true
		}
	}

	var cmd tea.Cmd
	switch {
	case key.Matches(msg, keys.ShareImage):
		m = m.openFileExplorer()
	case key.Matches(msg, keys.Invite):
		m = m.openQRCodeDialog()
	case key.Matches(msg, keys.ToggleCompanion):
		m.state.chat.companion.hidden = !c.companion.hidden
		m = m.relayoutChat()
	case key.Matches(msg, keys.AIRewrite):
		m, cmd = m.rewriteDraft()
	case key.Matches(msg, keys.LeaveRoom):
		m = m.openLeaveRoomDialog()
	case key.Matches(msg, keys.NewJoinCode):
		if !c.isRoomOwner {
			m, cmd = m.toast(theme.ToneWarning, "Only the room owner can change the join code")
		} else {
			m = m.openNewJoinCodeDialog()
		}
	case key.Matches(msg, keys.ToggleSearch):
		if c.members.searching {
			m = m.closeMemberSearch()
		} else {
			m, cmd = m.openMemberSearch()
		}
	case key.Matches(msg, keys.EditMessages):
		m, cmd = m.enterEditMode()
	case key.Matches(msg, keys.SearchMessages) && !c.members.searching:
		m, cmd = m.openMessageSearch()
	case key.Matches(msg, keys.Back) && c.social.replyTo != "" && !c.members.searching:
		m.state.chat.social.replyTo = ""
	case key.Matches(msg, keys.Enter) && !c.members.searching:
		m, cmd = m.sendMessage()
	case key.Matches(msg, keys.JumpToBottom):
		m.state.chat.viewport.GotoBottom()
		m.state.chat.unread = 0
		m, cmd = m.markReadSoon()
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// updateChatInputs passes msg to the focused input and, outside edit mode, to
// the message list.
func (m model) updateChatInputs(msg tea.Msg) (model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	if m.state.chat.members.searching {
		m, cmd = m.updateMemberSearchInput(msg)
	} else {
		before := m.state.chat.messageInput.Value()
		m.state.chat.messageInput, cmd = m.state.chat.messageInput.Update(msg)
		cmds = append(cmds, cmd)
		m, cmd = m.noteTyping(before)
	}
	cmds = append(cmds, cmd)

	if !m.state.chat.edit.active {
		m.state.chat.viewport, cmd = m.state.chat.viewport.Update(msg)
		cmds = append(cmds, cmd)
		if m.state.chat.viewport.AtBottom() {
			m.state.chat.unread = 0
			m, cmd = m.markReadSoon()
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// ---------------------------------------------------------------------------
// Edit mode

func (m model) enterEditMode() (model, tea.Cmd) {
	selected := len(m.state.chat.messages) - 1
	if selected < 0 {
		return m.toast(theme.ToneInfo, "No messages yet")
	}
	m.state.chat.edit = editState{active: true, selected: selected}
	return m.refreshMessages(), nil
}

func (m model) exitEditMode() model {
	m.state.chat.edit.active = false
	m.state.chat.edit.selected = -1
	return m.refreshMessages()
}

// editModeUpdate handles keys while a message is selected.
func (m model) editModeUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	c := m.state.chat
	n := len(c.messages)
	valid := c.edit.selected >= 0 && c.edit.selected < n
	var selected apisdk.MessageResponse
	if valid {
		selected = c.messages[c.edit.selected]
	}
	own := valid && m.isCurrentUser(selected.UserID)

	switch key := msg.String(); key {
	case "up", "k":
		if n > 0 {
			m.state.chat.edit.selected = min(max(c.edit.selected-1, 0), n-1)
		}
		return m.refreshMessages(), nil
	case "down", "j":
		if n > 0 {
			m.state.chat.edit.selected = min(c.edit.selected+1, n-1)
		}
		return m.refreshMessages(), nil
	case "delete", "ctrl+d":
		switch {
		case own:
			m.state.chat.edit.messageID = selected.ID
			return m.openDeleteMessageDialog(selected.Content), nil
		case valid:
			return m.toastNotOwn("delete")
		}
	case "enter":
		switch {
		case own && isImageURL(selected.Content):
			return m.toast(theme.ToneInfo, "Images can't be edited, only deleted.")
		case own:
			m.state.chat.edit.messageID = selected.ID
			return m.openEditMessageDialog(selected.Content), nil
		case valid:
			return m.toastNotOwn("edit")
		}
	case "r":
		if valid {
			m.state.chat.social.replyTo = selected.ID
			return m.exitEditMode(), nil
		}
	case "0":
		if valid {
			for _, r := range selected.Reactions {
				if m.isCurrentUser(r.UserID) {
					return m, m.react(c.edit.selected, r.Emoji) // toggles it off
				}
			}
		}
	case "1", "2", "3", "4", "5", "6":
		if valid {
			return m, m.react(c.edit.selected, quickReactions[key[0]-'1'])
		}
	case "esc", "ctrl+e":
		return m.exitEditMode(), nil
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Dialogs

// chatDialogUpdate handles keys while a dialog is open on the chat page.
func (m model) chatDialogUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	switch m.state.dialog.kind {
	case dialogFileExplorer:
		return m.fileExplorerUpdate(msg)
	case dialogQRCode:
		if msg.String() == "c" {
			code := m.state.dialog.content
			m, cmd := m.closeDialog().toast(theme.ToneSuccess, "Invite code copied. It unlocks the room: share it only with people you invite.")
			return m, tea.Batch(tea.SetClipboard(code), cmd)
		}
		return m.closeDialog(), nil
	case dialogAlert:
		switch msg.String() {
		case "enter", "esc", "space", "o", "O":
			return m.closeDialog(), nil
		}
		return m, nil
	case dialogRoomExpired:
		switch msg.String() {
		case "enter", "esc", "space", "o", "O", "y", "Y":
			return m.closeRoom(m.state.chat.session)
		}
		return m, nil
	case dialogEditMessage:
		return m.editMessageDialogUpdate(msg)
	}

	switch m.confirmAnswer(msg) {
	case answerToggle:
		m.state.dialog.focus = 1 - m.state.dialog.focus
		return m, nil
	case answerNo:
		m.state.chat.members.kickUserID = ""
		return m.closeDialog(), nil
	case answerYes:
		return m.confirmDialog()
	}
	return m, nil
}

// confirmDialog carries out the action of the open confirm dialog.
func (m model) confirmDialog() (model, tea.Cmd) {
	kind := m.state.dialog.kind
	m = m.closeDialog()

	switch kind {
	case dialogNewJoinCode:
		return m, m.generateJoinCode()
	case dialogKickMember:
		userID := m.state.chat.members.kickUserID
		m.state.chat.members.kickUserID = ""
		return m, m.kickMember(userID)
	case dialogLeaveRoom:
		return m.leaveChat()
	case dialogDeleteMessage:
		m = m.exitEditMode()
		return m, m.deleteMessage(m.state.chat.edit.messageID)
	}
	return m, nil
}

func (m model) editMessageDialogUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		id := m.state.chat.edit.messageID
		content := m.state.chat.editInput.Value()
		m = m.closeDialog()
		if content == "" || content == m.messageContent(id) {
			return m, nil
		}
		return m, m.updateMessage(id, content)
	case "esc":
		return m.closeDialog().exitEditMode(), nil
	}
	var cmd tea.Cmd
	m.state.chat.editInput, cmd = m.state.chat.editInput.Update(msg)
	return m, cmd
}
