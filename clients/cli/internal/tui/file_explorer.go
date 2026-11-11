package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hilthontt/visper/clients/cli/internal/strfn"
)

// The file explorer is the dialog that picks an image to share (ctrl+u).

const (
	feWidth       = 60
	feHeight      = 22
	feHeaderLines = 4 // title + path + separator + padding
	feFooterLines = 3 // scroll indicator + separator + keybinds
)

type fileExplorerEntry struct {
	name  string
	isDir bool
}

type fileExplorerState struct {
	dir     string
	entries []fileExplorerEntry
	cursor  int
	err     string
}

// imageFileSelectedMsg reports the image picked in the file explorer.
type imageFileSelectedMsg struct {
	path string
}

// cd lists dir and makes it the current directory. On error the explorer
// stays where it was and shows the error.
func (fe *fileExplorerState) cd(dir string) error {
	entries, err := listImageDir(dir)
	if err != nil {
		fe.err = fmt.Sprintf("Cannot read %s: %v", dir, err)
		return err
	}
	*fe = fileExplorerState{dir: dir, entries: entries}
	return nil
}

func (m model) openFileExplorer() model {
	fe := fileExplorerState{dir: homeDir()}
	_ = fe.cd(fe.dir)
	m.state.chat.fileExplorer = fe
	return m.openDialog(dialogState{kind: dialogFileExplorer, title: "Select Image"})
}

func (m model) fileExplorerUpdate(msg tea.KeyPressMsg) (model, tea.Cmd) {
	fe := &m.state.chat.fileExplorer

	switch msg.String() {
	case "esc":
		return m.closeDialog(), nil
	case "~":
		_ = fe.cd(homeDir())
	case "up", "k":
		fe.cursor = max(fe.cursor-1, 0)
	case "down", "j":
		fe.cursor = max(min(fe.cursor+1, len(fe.entries)-1), 0)
	case "enter", "right", "l":
		if len(fe.entries) == 0 {
			break
		}
		selected := fe.entries[fe.cursor]
		switch {
		case selected.name == "..":
			m = m.fileExplorerUp()
		case selected.isDir:
			_ = fe.cd(filepath.Join(fe.dir, selected.name))
		default:
			path := filepath.Join(fe.dir, selected.name)
			return m.closeDialog(), func() tea.Msg { return imageFileSelectedMsg{path: path} }
		}
	case "left", "h", "backspace":
		m = m.fileExplorerUp()
	}
	return m, nil
}

// fileExplorerUp goes to the parent directory, with the one we came from
// highlighted.
func (m model) fileExplorerUp() model {
	fe := &m.state.chat.fileExplorer
	parent := filepath.Dir(fe.dir)
	if parent == fe.dir {
		return m // already at the root
	}
	from := filepath.Base(fe.dir)
	if fe.cd(parent) == nil {
		fe.cursor = max(slices.IndexFunc(fe.entries, func(e fileExplorerEntry) bool { return e.name == from }), 0)
	}
	return m
}

// listImageDir lists the subdirectories and image files of dir, directories
// first, with ".." on top unless dir is the root. Hidden entries are skipped.
func listImageDir(dir string) ([]fileExplorerEntry, error) {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs, imgs []fileExplorerEntry
	for _, e := range raw {
		switch {
		case strings.HasPrefix(e.Name(), "."):
		case e.IsDir():
			dirs = append(dirs, fileExplorerEntry{name: e.Name(), isDir: true})
		case isImageFile(e.Name()):
			imgs = append(imgs, fileExplorerEntry{name: e.Name()})
		}
	}

	var entries []fileExplorerEntry
	if filepath.Dir(dir) != dir {
		entries = append(entries, fileExplorerEntry{name: "..", isDir: true})
	}
	return append(append(entries, dirs...), imgs...), nil
}

func isImageFile(name string) bool {
	return slices.Contains(imageExtensions, strings.ToLower(filepath.Ext(name)))
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "/"
}

// fileSummary describes a file in one line: its name, size, modification time
// and permissions, or the target of a symlink. It is "" if the file can't be
// read.
func fileSummary(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return ""
		}
		return "Path: " + target
	}
	return strings.Join([]string{
		"Name: " + info.Name(),
		"Size: " + strfn.FormatFileSize(info.Size()),
		"Date Modified: " + info.ModTime().String(),
		"Permissions: " + info.Mode().String(),
	}, "  •  ")
}

// truncateLeft keeps the end of s, so a long path still shows its last part.
func truncateLeft(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	runes := []rune(s)
	return "…" + string(runes[len(runes)-(width-1):])
}

func (m model) fileExplorerView() string {
	fe := m.state.chat.fileExplorer
	dim := m.theme.TextSubtle()
	separator := dim.Render(strings.Repeat("─", feWidth-2))
	textWidth := feWidth - 4

	var sb strings.Builder
	line := func(s string) {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	// Header
	line(m.theme.TextBrand().Render("  ◆  ") + m.theme.TextAccent().Bold(true).Render("Share an image"))
	line(dim.Render("  " + truncateLeft(fe.dir, textWidth)))
	line(separator)

	if len(fe.entries) > 0 {
		selected := fe.entries[fe.cursor]
		path := filepath.Join(fe.dir, selected.name)
		if selected.name == ".." {
			path = filepath.Dir(fe.dir)
		}
		if summary := fileSummary(path); summary != "" {
			line(dim.Render("  " + truncate(summary, textWidth)))
		}
	}
	line(separator)

	if fe.err != "" {
		line(m.theme.TextError().Render("  ⚠  " + fe.err))
	}

	// Entries, in a window that keeps the cursor centred.
	listHeight := feHeight - feHeaderLines - feFooterLines
	total := len(fe.entries)
	start := max(fe.cursor-listHeight/2, 0)
	if start+listHeight > total {
		start = max(0, total-listHeight)
	}
	end := min(start+listHeight, total)

	if total == 0 {
		line(dim.Render("  (no images or subdirectories found)"))
	}
	for i := start; i < end; i++ {
		row := m.fileExplorerEntryView(fe.entries[i])
		if i == fe.cursor {
			row = m.theme.TextSecondary().Bold(true).Render("▌ ") + m.theme.Base().Background(m.theme.Surface()).Render(row)
		} else {
			row = "  " + row
		}
		line("  " + row)
	}

	if total > listHeight {
		line(dim.Render(fmt.Sprintf("  %d–%d of %d", start+1, end, total)))
	}

	line(separator)
	sb.WriteString("  ")
	sb.WriteString(m.keyHints([]keyHint{{"↑↓", "move"}, {"←→", "folders"}, {"enter", "send"}, {"~", "home"}, {"esc", "cancel"}}, feWidth-6))

	return m.theme.Base().
		Width(feWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Brand()).
		Padding(0, 1).
		Render(sb.String())
}

func (m model) fileExplorerEntryView(e fileExplorerEntry) string {
	switch {
	case e.name == "..":
		return "⬆  " + m.theme.TextBrand().Bold(true).Render(e.name+"/")
	case e.isDir:
		return "📁 " + m.theme.TextBrand().Bold(true).Render(e.name+"/")
	default:
		return "🖼  " + m.theme.TextAccent().Render(e.name)
	}
}
