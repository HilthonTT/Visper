package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
	"github.com/hilthontt/visper/sdk/go/option"
)

// Typing indicators, presence, read receipts, replies, reactions and message
// search. Reactions are encrypted like messages; typing, presence and read
// markers go over the room WebSocket and are never stored as messages.

// quickReactions are picked with 1–6 while a message is selected.
var quickReactions = []string{"👍", "😂", "😮", "😢", "🎉", "🔥"}

const (
	// typingSendInterval matches the server's relay limit.
	typingSendInterval = 2 * time.Second
	// typingShownFor is how long "x is typing…" stays without a new event.
	typingShownFor = 5 * time.Second
	// readDebounce batches read markers (e.g. during the history replay).
	readDebounce = time.Second
)

type typingMember struct {
	name  string
	until time.Time
}

// messageSearchState is ctrl+f: finding text in the loaded messages. They
// are encrypted on the server, so this is the only search there is.
type messageSearchState struct {
	active  bool
	input   textinput.Model
	matches []int // indexes into messages, oldest first
	cursor  int   // index into matches
}

// current is the index of the highlighted match, or -1.
func (s messageSearchState) current() int {
	if !s.active || len(s.matches) == 0 {
		return -1
	}
	return s.matches[s.cursor]
}

// socialState is the chat state for these features.
type socialState struct {
	// online is who is connected right now; nil until the server says.
	online map[string]bool
	typing map[string]typingMember
	// reads is the last message each member has read.
	reads map[string]string

	lastTypingSent time.Time
	lastReadSent   string
	readPending    bool

	// replyTo is the message the draft answers, if any.
	replyTo string
	search  messageSearchState
}

func newSocialState(searchInput textinput.Model) socialState {
	return socialState{
		typing: make(map[string]typingMember),
		reads:  make(map[string]string),
		search: messageSearchState{input: searchInput},
	}
}

// Room events for these features (see parseRoomEvent).
type (
	presenceMsg struct {
		online []string
	}
	memberTypingMsg struct {
		userID   string
		username string
	}
	readUpdateMsg struct {
		userID    string
		messageID string
	}
	messageReactionsMsg struct {
		messageID string
		reactions []apisdk.Reaction
	}
	typingExpiredMsg struct {
		session context.Context
	}
	readTickMsg struct {
		session context.Context
	}
)

// socialEventUpdate handles the events above; handled is false otherwise.
func (m model) socialEventUpdate(msg tea.Msg) (_ model, _ tea.Cmd, handled bool) {
	c := &m.state.chat
	switch msg := msg.(type) {
	case presenceMsg:
		c.social.online = make(map[string]bool, len(msg.online))
		for _, id := range msg.online {
			c.social.online[id] = true
		}
		return m, m.waitForRoomEvent(), true

	case memberTypingMsg:
		if m.isCurrentUser(msg.userID) {
			return m, m.waitForRoomEvent(), true
		}
		c.social.typing[msg.userID] = typingMember{name: msg.username, until: time.Now().Add(typingShownFor)}
		session := c.session
		expire := tea.Tick(typingShownFor+100*time.Millisecond, func(time.Time) tea.Msg {
			return typingExpiredMsg{session: session}
		})
		return m, tea.Batch(expire, m.waitForRoomEvent()), true

	case typingExpiredMsg:
		if msg.session == c.session {
			now := time.Now()
			for id, t := range c.social.typing {
				if now.After(t.until) {
					delete(c.social.typing, id)
				}
			}
		}
		return m, nil, true

	case readUpdateMsg:
		c.social.reads[msg.userID] = msg.messageID
		return m.refreshMessages(), m.waitForRoomEvent(), true

	case messageReactionsMsg:
		for i := range c.messages {
			if c.messages[i].ID == msg.messageID {
				c.messages[i].Reactions = msg.reactions
				break
			}
		}
		return m.refreshMessages(), m.waitForRoomEvent(), true

	case readTickMsg:
		if msg.session != c.session {
			return m, nil, true
		}
		c.social.readPending = false
		return m, m.sendRead(), true
	}
	return m, nil, false
}

// ---------------------------------------------------------------------------
// Typing

// noteTyping tells the room the user is typing when the draft changed,
// at most every typingSendInterval.
func (m model) noteTyping(before string) (model, tea.Cmd) {
	c := &m.state.chat
	after := c.messageInput.Value()
	if after == before || after == "" || c.ws == nil || time.Since(c.social.lastTypingSent) < typingSendInterval {
		return m, nil
	}
	c.social.lastTypingSent = time.Now()
	ws := c.ws
	return m, func() tea.Msg {
		_ = ws.SendTyping()
		return nil
	}
}

// typingText is "alice is typing…" for the members typing right now.
func (s socialState) typingText() string {
	now := time.Now()
	var names []string
	for _, t := range s.typing {
		if now.Before(t.until) {
			names = append(names, t.name)
		}
	}
	slices.Sort(names)
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " is typing…"
	case 2:
		return names[0] + " and " + names[1] + " are typing…"
	}
	return fmt.Sprintf("%d people are typing…", len(names))
}

// ---------------------------------------------------------------------------
// Read receipts

// markReadSoon sends a read marker for the latest message shortly, once.
func (m model) markReadSoon() (model, tea.Cmd) {
	c := &m.state.chat
	if c.social.readPending || len(c.messages) == 0 {
		return m, nil
	}
	c.social.readPending = true
	session := c.session
	return m, tea.Tick(readDebounce, func(time.Time) tea.Msg { return readTickMsg{session: session} })
}

// sendRead tells the room the user has read the latest message, if they can
// see it and haven't said so already.
func (m model) sendRead() tea.Cmd {
	c := &m.state.chat
	if c.ws == nil || len(c.messages) == 0 || !c.viewport.AtBottom() {
		return nil
	}
	last := c.messages[len(c.messages)-1].ID
	if last == c.social.lastReadSent {
		return nil
	}
	c.social.lastReadSent = last
	ws := c.ws
	return func() tea.Msg {
		_ = ws.SendRead(last)
		return nil
	}
}

// seenBy lists who has read up to msg, other than its author and the user.
func (m model) seenBy(msg apisdk.MessageResponse) []string {
	var names []string
	for userID, messageID := range m.state.chat.social.reads {
		if messageID != msg.ID || userID == msg.UserID || m.isCurrentUser(userID) {
			continue
		}
		if name := m.state.chat.members.name(userID); name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ---------------------------------------------------------------------------
// Replies and reactions

// replyTarget is the message being replied to, if it is still there.
func (m model) replyTarget() (apisdk.MessageResponse, bool) {
	id := m.state.chat.social.replyTo
	if id == "" {
		return apisdk.MessageResponse{}, false
	}
	for _, msg := range m.state.chat.messages {
		if msg.ID == id {
			return msg, true
		}
	}
	return apisdk.MessageResponse{}, false
}

// quote is a one-line summary of a message for reply previews.
func quote(msg apisdk.MessageResponse, width int) string {
	text := msg.Content
	if isImageURL(text) {
		text = "[image]"
	}
	text = strings.Join(strings.Fields(text), " ")
	return truncate(msg.Username+": "+text, width)
}

// react sets the user's reaction to the message at index i; picking their
// current reaction again removes it.
func (m model) react(i int, emoji string) tea.Cmd {
	c := m.state.chat
	if i < 0 || i >= len(c.messages) {
		return nil
	}
	msg := c.messages[i]
	for _, r := range msg.Reactions {
		if m.isCurrentUser(r.UserID) && r.Emoji == emoji {
			emoji = "" // toggle off
		}
	}
	return m.roomRequest("react", func(ctx context.Context, cl *apisdk.Client, roomID string, opts []option.RequestOption) error {
		var err error
		if emoji == "" {
			_, err = cl.Message.RemoveReaction(ctx, roomID, msg.ID, opts...)
		} else {
			_, err = cl.Message.SetReaction(ctx, roomID, msg.ID, emoji, opts...)
		}
		return err
	})
}

// reactionsLine is "👍 2  🎉 1" for a message, the user's own reaction
// highlighted; "" when it has none.
func (m model) reactionsLine(msg apisdk.MessageResponse) string {
	if len(msg.Reactions) == 0 {
		return ""
	}
	var order []string
	counts := map[string]int{}
	mine := ""
	for _, r := range msg.Reactions {
		if counts[r.Emoji] == 0 {
			order = append(order, r.Emoji)
		}
		counts[r.Emoji]++
		if m.isCurrentUser(r.UserID) {
			mine = r.Emoji
		}
	}
	parts := make([]string, len(order))
	for i, e := range order {
		text := fmt.Sprintf("%s %d", e, counts[e])
		if e == mine {
			parts[i] = m.theme.Pill(m.theme.Surface(), m.theme.Brand()).Render(text)
		} else {
			parts[i] = m.theme.TextSubtle().Render(" " + text + " ")
		}
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Message search

func (m model) openMessageSearch() (model, tea.Cmd) {
	s := &m.state.chat.social.search
	s.active = true
	s.input.SetValue("")
	s.matches, s.cursor = nil, 0
	m.state.chat.messageInput.Blur()
	return m, s.input.Focus()
}

func (m model) closeMessageSearch() model {
	s := &m.state.chat.social.search
	s.active = false
	s.input.Blur()
	s.matches, s.cursor = nil, 0
	m.state.chat.messageInput.Focus()
	return m.refreshMessages()
}

// findMessages recomputes the matches for the search query, highlighting the
// newest one.
func (m model) findMessages() model {
	c := &m.state.chat
	s := &c.social.search
	q := strings.ToLower(strings.TrimSpace(s.input.Value()))
	s.matches, s.cursor = nil, 0
	if q != "" {
		for i, msg := range c.messages {
			if isImageURL(msg.Content) {
				continue
			}
			if strings.Contains(strings.ToLower(msg.Content), q) || strings.Contains(strings.ToLower(msg.Username), q) {
				s.matches = append(s.matches, i)
			}
		}
		s.cursor = max(len(s.matches)-1, 0)
	}
	return m.refreshMessages()
}

// messageSearchKeyUpdate handles keys while searching messages.
func (m model) messageSearchKeyUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	s := &m.state.chat.social.search
	switch msg.String() {
	case "esc", "ctrl+f":
		return m.closeMessageSearch(), nil
	case "up", "enter":
		if n := len(s.matches); n > 0 {
			s.cursor = (s.cursor - 1 + n) % n // older
		}
		return m.refreshMessages(), nil
	case "down":
		if n := len(s.matches); n > 0 {
			s.cursor = (s.cursor + 1) % n // newer
		}
		return m.refreshMessages(), nil
	}
	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		m = m.findMessages()
	}
	return m, cmd
}

// searchStatus is "2/5" or "no matches" for the status line.
func (s messageSearchState) status() string {
	switch {
	case strings.TrimSpace(s.input.Value()) == "":
		return "type to search"
	case len(s.matches) == 0:
		return "no matches"
	}
	return fmt.Sprintf("%d/%d", s.cursor+1, len(s.matches))
}

// toastNotOwn explains that only your own messages can be changed.
func (m model) toastNotOwn(action string) (model, tea.Cmd) {
	return m.toast(theme.ToneInfo, "You can only "+action+" your own messages. Press r to reply or 1-6 to react.")
}
