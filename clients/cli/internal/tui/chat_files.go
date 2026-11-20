package tui

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
	"github.com/hilthontt/visper/clients/cli/internal/tui/theme"
	apisdk "github.com/hilthontt/visper/sdk/go"
)

// The files dialog (ctrl+o) lists the files shared in the room and deletes
// them. The server stores only ciphertext under a meaningless name, so a
// file's real name comes from the (decrypted) image message pointing to it.

const (
	filesWidth      = 64
	filesListHeight = 12
)

type roomFilesState struct {
	loading bool
	err     string
	files   []apisdk.FileResponse
	cursor  int
	// deleteID is the file the delete dialog is for.
	deleteID string
}

type (
	roomFilesLoadedMsg struct {
		files   []apisdk.FileResponse
		err     error
		session context.Context
	}
	roomFileDeletedMsg struct {
		err     error
		session context.Context
	}
)

// openRoomFiles opens the files dialog and loads the list.
func (m model) openRoomFiles() (model, tea.Cmd) {
	m.state.chat.files = roomFilesState{loading: true}
	m = m.openDialog(dialogState{kind: dialogRoomFiles, title: "Files in this room"})
	return m, m.loadRoomFiles()
}

func (m model) loadRoomFiles() tea.Cmd {
	if m.state.chat.room == nil {
		return nil
	}
	client, ctx, roomID, opts, session := m.client, m.ctx, m.state.chat.room.ID, m.requestOpts(), m.state.chat.session
	return func() tea.Msg {
		files, err := client.File.List(ctx, roomID, opts...)
		return roomFilesLoadedMsg{files: files, err: err, session: session}
	}
}

func (m model) deleteRoomFile(fileID string) tea.Cmd {
	if m.state.chat.room == nil {
		return nil
	}
	client, ctx, roomID, opts, session := m.client, m.ctx, m.state.chat.room.ID, m.requestOpts(), m.state.chat.session
	return func() tea.Msg {
		return roomFileDeletedMsg{err: client.File.Delete(ctx, roomID, fileID, opts...), session: session}
	}
}

func (m model) roomFilesLoadedUpdate(msg roomFilesLoadedMsg) (model, tea.Cmd) {
	if msg.session != m.state.chat.session {
		return m, nil
	}
	f := &m.state.chat.files
	f.loading = false
	if msg.err != nil {
		f.err = fmt.Sprintf("Couldn't load the files: %v", msg.err)
		return m, nil
	}
	f.err = ""
	f.files = msg.files
	slices.SortStableFunc(f.files, func(a, b apisdk.FileResponse) int {
		return b.CreatedAt.Compare(a.CreatedAt) // newest first
	})
	f.cursor = min(f.cursor, max(len(f.files)-1, 0))
	return m, nil
}

func (m model) roomFileDeletedUpdate(msg roomFileDeletedMsg) (model, tea.Cmd) {
	if msg.session != m.state.chat.session {
		return m, nil
	}
	if msg.err != nil {
		return m.toast(theme.ToneDanger, fmt.Sprintf("Couldn't delete the file: %v", msg.err))
	}
	m, toast := m.toast(theme.ToneSuccess, "File deleted. Messages pointing to it can no longer show it.")
	if m.state.dialog.open && m.state.dialog.kind == dialogRoomFiles {
		m.state.chat.files.loading = true
		return m, tea.Batch(toast, m.loadRoomFiles())
	}
	return m, toast
}

// canDeleteFile reports whether the user may delete f: their own files, or
// any file in a room they own.
func (m model) canDeleteFile(f apisdk.FileResponse) bool {
	return m.state.chat.isRoomOwner || m.isCurrentUser(f.Uploader.ID)
}

func (m model) roomFilesUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	f := &m.state.chat.files
	switch msg.String() {
	case "esc", "q", "ctrl+o":
		return m.closeDialog(), nil
	case "up", "k":
		f.cursor = max(f.cursor-1, 0)
	case "down", "j":
		f.cursor = max(min(f.cursor+1, len(f.files)-1), 0)
	case "r":
		f.loading = true
		return m, m.loadRoomFiles()
	case "d", "delete", "x":
		if f.loading || len(f.files) == 0 {
			return m, nil
		}
		file := f.files[f.cursor]
		if !m.canDeleteFile(file) {
			return m.toast(theme.ToneWarning, "Only the uploader or the room owner can delete a file")
		}
		f.deleteID = file.ID
		return m.openDialog(dialogState{
			kind:    dialogDeleteFile,
			tone:    theme.ToneDanger,
			title:   "Delete this file?",
			content: fmt.Sprintf("“%s” is removed from the server for everyone. Messages pointing to it stay, without the file.", m.fileDisplayName(file)),
		}), nil
	}
	return m, nil
}

// reopenRoomFiles goes back to the files dialog from its delete dialog.
func (m model) reopenRoomFiles() model {
	m.state.chat.files.deleteID = ""
	return m.openDialog(dialogState{kind: dialogRoomFiles, title: "Files in this room"})
}

// fileDisplayName is the name the room's messages give f, or a placeholder.
func (m model) fileDisplayName(f apisdk.FileResponse) string {
	for _, msg := range m.state.chat.messages {
		if id, name, ok := sharedFileRef(msg.Content); ok && id == f.ID && name != "" {
			return name
		}
	}
	return "encrypted file"
}

// sharedFileRef parses a message pointing to an uploaded file
// ("https://host/api/v1/d/<room>/<file ID>.enc#<name>") into the file's ID
// and name.
func sharedFileRef(content string) (fileID, name string, ok bool) {
	file, frag, _ := strings.Cut(strings.TrimSpace(content), "#")
	u, err := url.Parse(file)
	if err != nil || !isHTTPURL(file) || !strings.HasSuffix(u.Path, encryptedFileExt) {
		return "", "", false
	}
	fileID = strings.TrimSuffix(path.Base(u.Path), encryptedFileExt)
	if fileID == "" {
		return "", "", false
	}
	if n, err := url.PathUnescape(frag); err == nil {
		name = n
	}
	return fileID, name, true
}

// fileUploaderName is how the files dialog names f's uploader.
func (m model) fileUploaderName(f apisdk.FileResponse) string {
	switch {
	case m.isCurrentUser(f.Uploader.ID):
		return "you"
	case f.Uploader.Username != "":
		return f.Uploader.Username
	}
	if name := m.state.chat.members.name(f.Uploader.ID); name != "" {
		return name
	}
	return "someone who left"
}

func (m model) roomFilesView() string {
	f := m.state.chat.files
	width := min(filesWidth, max(m.viewportWidth-2, 30))
	inner := width - 4
	dim := m.theme.TextSubtle()

	lines := []string{
		m.theme.TextBrand().Render("◆  ") + m.theme.TextAccent().Bold(true).Render("Files in this room"),
		dim.Render(truncate("Only ciphertext is on the server; names come from the room's messages.", inner)),
		m.rule(inner),
	}

	switch {
	case f.loading && len(f.files) == 0:
		lines = append(lines, dim.Render("loading…"))
	case f.err != "":
		lines = append(lines, m.theme.TextError().Render(truncate("⚠  "+f.err, inner)))
	case len(f.files) == 0:
		lines = append(lines, dim.Render("No files shared yet. ctrl+u shares an image."))
	}

	total := len(f.files)
	start := max(f.cursor-filesListHeight/2, 0)
	if start+filesListHeight > total {
		start = max(0, total-filesListHeight)
	}
	end := min(start+filesListHeight, total)
	for i := start; i < end; i++ {
		file := f.files[i]
		meta := fmt.Sprintf("%s · %s · %s", strfn.FormatFileSize(file.Size), m.fileUploaderName(file), fileAge(file.CreatedAt))
		nameWidth := max(inner-lipgloss.Width(meta)-4, 8)
		name := truncate(m.fileDisplayName(file), nameWidth)
		row := spread(inner-2, m.theme.TextAccent().Render(name), dim.Render(meta))
		if i == f.cursor {
			row = m.theme.TextSecondary().Bold(true).Render("▌ ") + m.theme.Base().Background(m.theme.Surface()).Render(row)
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
	}
	if total > filesListHeight {
		lines = append(lines, dim.Render(fmt.Sprintf("%d–%d of %d", start+1, end, total)))
	}

	lines = append(lines, m.rule(inner),
		m.keyHints([]keyHint{{"↑↓", "move"}, {"d", "delete"}, {"r", "refresh"}, {"esc", "close"}}, inner))

	return m.theme.Base().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

// fileAge says how long ago t was, roughly.
func fileAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
