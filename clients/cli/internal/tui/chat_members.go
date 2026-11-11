package tui

import (
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	"github.com/hilthontt/visper/clients/cli/internal/utils"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// memberSearchTimeout bounds a fuzzy member search.
const memberSearchTimeout = 500 * time.Millisecond

// memberState is the member list in the left panel and its search.
type memberState struct {
	list []apisdk.UserResponse
	// filtered holds the indices in list of the members shown, best search
	// match first.
	filtered  []int
	cursor    int // highlighted row of the search results
	searching bool
	search    textinput.Model
	// kickUserID is the member the kick dialog is for.
	kickUserID string
}

// memberSearchResultMsg lists the IDs of the members matching query, best
// match first.
type memberSearchResultMsg struct {
	query string
	ids   []string
}

func newMemberState(search textinput.Model, members []apisdk.UserResponse) memberState {
	search.Prompt = "/ "
	s := memberState{list: members, search: search}
	s.resetFilter()
	return s
}

// resetFilter shows every member.
func (s *memberState) resetFilter() {
	s.filtered = make([]int, len(s.list))
	for i := range s.filtered {
		s.filtered[i] = i
	}
	s.clampCursor()
}

func (s *memberState) clampCursor() {
	s.cursor = min(s.cursor, max(len(s.filtered)-1, 0))
}

// name returns the username of a member, or "" if they are not in the list.
func (s memberState) name(userID string) string {
	if i := s.index(userID); i >= 0 {
		return s.list[i].Username
	}
	return ""
}

func (s memberState) index(userID string) int {
	return slices.IndexFunc(s.list, func(u apisdk.UserResponse) bool { return u.ID == userID })
}

// add appends a member unless they are already listed; it reports whether
// they were added.
func (s *memberState) add(member apisdk.UserResponse) bool {
	if s.index(member.ID) >= 0 {
		return false
	}
	s.list = append(s.list, member)
	if !s.searching || s.search.Value() == "" {
		s.filtered = append(s.filtered, len(s.list)-1)
	}
	return true
}

// remove drops a member, keeping an active search filter (and its highlighted
// row) pointing at the same people.
func (s *memberState) remove(userID string) {
	idx := s.index(userID)
	if idx < 0 {
		return
	}
	s.list = append(s.list[:idx:idx], s.list[idx+1:]...)

	filtered := make([]int, 0, len(s.filtered))
	for _, i := range s.filtered {
		switch {
		case i == idx:
			continue
		case i > idx:
			i--
		}
		filtered = append(filtered, i)
	}
	s.filtered = filtered
	s.clampCursor()
}

// applySearchResult filters the list by a search result. The matches are
// mapped back by ID: the list may have changed since the search snapshot, and
// stale indices would select (and kick) the wrong member.
func (s *memberState) applySearchResult(ids []string) {
	pos := make(map[string]int, len(s.list))
	for i, p := range s.list {
		pos[p.ID] = i
	}
	s.filtered = make([]int, 0, len(ids))
	for _, id := range ids {
		if i, ok := pos[id]; ok {
			s.filtered = append(s.filtered, i)
		}
	}
}

// highlighted returns the member on the highlighted search row.
func (s memberState) highlighted() (apisdk.UserResponse, bool) {
	if len(s.filtered) == 0 {
		return apisdk.UserResponse{}, false
	}
	idx := s.filtered[min(s.cursor, len(s.filtered)-1)]
	if idx < 0 || idx >= len(s.list) {
		return apisdk.UserResponse{}, false
	}
	return s.list[idx], true
}

func (m model) openMemberSearch() (model, tea.Cmd) {
	if m.chatLeftWidth() == 0 {
		return m.toast(theme.ToneInfo, "Widen the window to see the member list")
	}
	s := &m.state.chat.members
	s.searching = true
	s.cursor = 0
	s.search.Focus()
	m.state.chat.messageInput.Blur()
	return m, nil
}

func (m model) closeMemberSearch() model {
	s := &m.state.chat.members
	s.searching = false
	s.search.Blur()
	s.search.SetValue("")
	s.cursor = 0
	s.resetFilter()
	m.state.chat.messageInput.Focus()
	return m
}

// memberSearchKeyUpdate handles the keys that act on the search results;
// handled is false for keys that go to the search field.
func (m model) memberSearchKeyUpdate(msg tea.KeyPressMsg) (_ model, _ tea.Cmd, handled bool) {
	s := &m.state.chat.members
	switch msg.String() {
	case "up":
		s.cursor = max(s.cursor-1, 0)
		return m, nil, true
	case "down":
		s.cursor = min(s.cursor+1, max(len(s.filtered)-1, 0))
		return m, nil, true
	case "enter":
		mm, cmd := m.kickHighlightedMember()
		return mm, cmd, true
	case "esc":
		return m.closeMemberSearch(), nil, true
	}
	return m, nil, false
}

// updateMemberSearchInput passes msg to the search field and searches again
// if the query changed.
func (m model) updateMemberSearchInput(msg tea.Msg) (model, tea.Cmd) {
	s := &m.state.chat.members
	before := s.search.Value()
	var cmd tea.Cmd
	s.search, cmd = s.search.Update(msg)
	if q := s.search.Value(); q != before {
		return m, tea.Batch(cmd, m.searchMembers(q))
	}
	return m, cmd
}

// searchMembers fuzzy-matches query against the member names off the Update
// loop.
func (m model) searchMembers(query string) tea.Cmd {
	// Snapshot the members now; the command runs on another goroutine.
	list := m.state.chat.members.list
	names := make([]string, len(list))
	ids := make([]string, len(list))
	for i, p := range list {
		names[i], ids[i] = p.Username, p.ID
	}
	return func() tea.Msg {
		if query == "" {
			return memberSearchResultMsg{query: query}
		}
		results, err := utils.FzfSearch(query, names, memberSearchTimeout)
		if err != nil {
			return memberSearchResultMsg{query: query}
		}
		matched := make([]string, 0, len(results))
		for _, r := range results {
			if i := int(r.HayIndex); i >= 0 && i < len(ids) {
				matched = append(matched, ids[i])
			}
		}
		return memberSearchResultMsg{query: query, ids: matched}
	}
}

func (m model) kickHighlightedMember() (model, tea.Cmd) {
	selected, ok := m.state.chat.members.highlighted()
	if !ok {
		return m, nil
	}
	if !m.state.chat.isRoomOwner {
		return m.toast(theme.ToneInfo, "Only the room owner can kick members")
	}
	if m.isCurrentUser(selected.ID) {
		return m.toast(theme.ToneInfo, "You can't kick yourself. Use ctrl+b to leave.")
	}
	m.state.chat.members.kickUserID = selected.ID
	return m.openKickMemberDialog(selected.Username), nil
}
