package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

func isImageFile(name string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(name))]
}

type fileExplorerEntry struct {
	name  string
	isDir bool
}

type fileExplorerState struct {
	currentDir string
	entries    []fileExplorerEntry
	cursor     int
	err        string
}

type imageFileSelectedMsg struct {
	path string
}

type fileExplorerClosedMsg struct{}

func loadDir(dir string) ([]fileExplorerEntry, error) {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs, imgs []fileExplorerEntry
	for _, e := range raw {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, fileExplorerEntry{name: e.Name(), isDir: true})
		} else if isImageFile(e.Name()) {
			imgs = append(imgs, fileExplorerEntry{name: e.Name(), isDir: false})
		}
	}

	// Parent directory shortcut (skip at filesystem root)
	entries := []fileExplorerEntry{}
	if dir != "/" {
		entries = append(entries, fileExplorerEntry{name: "..", isDir: true})
	}
	entries = append(entries, dirs...)
	entries = append(entries, imgs...)
	return entries, nil
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

func (m model) openFileExplorer() model {
	dir := homeDir()
	entries, err := loadDir(dir)

	fe := fileExplorerState{
		currentDir: dir,
		entries:    entries,
		cursor:     0,
	}
	if err != nil {
		fe.err = fmt.Sprintf("Cannot read directory: %v", err)
	}

	m.state.chat.fileExplorer = fe
	m.state.notify = notifyState{
		open:          true,
		title:         "Select Image",
		confirmAction: FileExplorerAction,
	}
	return m
}

func (m model) fileExplorerUpdate(msg tea.KeyMsg) (model, tea.Cmd) {
	fe := &m.state.chat.fileExplorer

	switch msg.String() {
	case "~":
		dir := homeDir()
		entries, err := loadDir(dir)
		if err != nil {
			fe.err = fmt.Sprintf("Cannot read home: %v", err)
			break
		}
		fe.currentDir = dir
		fe.entries = entries
		fe.cursor = 0
		fe.err = ""
	case "esc":
		m = m.closeModal()
		return m, func() tea.Msg {
			return fileExplorerClosedMsg{}
		}

	case "up", "k":
		if fe.cursor > 0 {
			fe.cursor--
		}

	case "down", "j":
		if fe.cursor < len(fe.entries)-1 {
			fe.cursor++
		}

	case "enter", "right", "l":
		if len(fe.entries) == 0 {
			break
		}
		selected := fe.entries[fe.cursor]

		if selected.isDir {
			// Navigate into the directory
			var next string
			if selected.name == ".." {
				next = filepath.Dir(fe.currentDir)
			} else {
				next = filepath.Join(fe.currentDir, selected.name)
			}

			entries, err := loadDir(next)
			if err != nil {
				fe.err = fmt.Sprintf("Cannot read %s: %v", next, err)
				break
			}
			fe.currentDir = next
			fe.entries = entries
			fe.cursor = 0
			fe.err = ""

		} else {
			// Image file selected – close modal and emit the message
			selectedPath := filepath.Join(fe.currentDir, selected.name)
			m = m.closeModal()
			return m, func() tea.Msg {
				return imageFileSelectedMsg{path: selectedPath}
			}
		}

	case "left", "h", "backspace":
		// Go up one directory
		parent := filepath.Dir(fe.currentDir)
		if parent == fe.currentDir {
			break // already at root
		}
		entries, err := loadDir(parent)
		if err != nil {
			fe.err = fmt.Sprintf("Cannot read %s: %v", parent, err)
			break
		}

		prevDirName := filepath.Base(fe.currentDir)

		fe.currentDir = parent
		fe.entries = entries
		fe.cursor = 0

		// Find the child dir we came from and restore cursor ther
		for i, e := range entries {
			if e.name == prevDirName {
				fe.cursor = i
				break
			}
		}

		fe.err = ""
	}

	return m, nil
}

const (
	feModalWidth  = 60
	feModalHeight = 22
	feHeaderLines = 4 // title + path + separator + padding
	feFooterLines = 3 // scroll indicator + separator + keybinds
)

func (m model) RenderFileExplorerModal() string {
	fe := m.state.chat.fileExplorer

	borderStyle := m.theme.Base().
		Width(feModalWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Highlight()).
		Padding(0, 1)

	titleStyle := m.theme.TextBrand().Bold(true)
	dimStyle := m.theme.TextBody().Faint(true)
	dirStyle := m.theme.TextAccent().Bold(true)
	fileStyle := m.theme.TextBody()
	cursorStyle := m.theme.Base().Foreground(m.theme.Highlight()).Bold(true)
	errorStyle := m.theme.Base().Foreground(lipgloss.Color("#EF4444"))

	// Header
	sb := strings.Builder{}

	title := titleStyle.Render("  Select Image to Upload")
	sb.WriteString(title)
	sb.WriteString("\n")

	// Truncate the path if it's too long
	dirPath := fe.currentDir
	maxPathLen := feModalWidth - 4
	if utf8.RuneCountInString(dirPath) > maxPathLen {
		runes := []rune(dirPath)
		dirPath = "…" + string(runes[len(runes)-(maxPathLen-1):])
	}
	sb.WriteString(dimStyle.Render("  " + dirPath))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(strings.Repeat("─", feModalWidth-2)))
	sb.WriteString("\n")

	if len(fe.entries) > 0 {
		selected := fe.entries[fe.cursor]
		selectedPath := filepath.Join(fe.currentDir, selected.name)
		if selected.name == ".." {
			selectedPath = filepath.Dir(fe.currentDir)
		}

		meta := GetMetadata(selectedPath, false, nil) // pass your et instance instead of nil if you have one
		data := meta.GetData()

		// Pick the fields you care about for the header
		wantedKeys := []string{"Name", "Size", "DateModified", "Permissions"}
		var metaParts []string
		for _, key := range wantedKeys {
			if val, err := meta.GetValue(key); err == nil {
				metaParts = append(metaParts, fmt.Sprintf("%s: %s", key, val))
			}
		}

		// Fallback: just take first 3 fields if key names don't match
		if len(metaParts) == 0 {
			limit := min(3, len(data))
			for _, pair := range data[:limit] {
				metaParts = append(metaParts, fmt.Sprintf("%s: %s", pair[0], pair[1]))
			}
		}

		if len(metaParts) > 0 {
			metaLine := strings.Join(metaParts, "  •  ")
			// Truncate if too long
			maxLen := feModalWidth - 4
			if utf8.RuneCountInString(metaLine) > maxLen {
				runes := []rune(metaLine)
				metaLine = string(runes[:maxLen-1]) + "…"
			}
			sb.WriteString(dimStyle.Render("  " + metaLine))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(dimStyle.Render(strings.Repeat("─", feModalWidth-2)))
	sb.WriteString("\n")

	// Error Banner
	if fe.err != "" {
		sb.WriteString(errorStyle.Render("  ⚠  " + fe.err))
		sb.WriteString("\n")
	}

	// Entry list
	listHeight := feModalHeight - feHeaderLines - feFooterLines
	total := len(fe.entries)

	// Windowing: keep cursor visible
	// Better: keep cursor centered
	windowStart := max(fe.cursor-listHeight/2, 0)
	if windowStart+listHeight > total {
		windowStart = max(0, total-listHeight)
	}
	windowEnd := min(windowStart+listHeight, total)

	if total == 0 {
		sb.WriteString(dimStyle.Render("  (no images or subdirectories found)"))
		sb.WriteString("\n")
	}

	for i := windowStart; i < windowEnd; i++ {
		entry := fe.entries[i]

		// Icon + name styling
		var icon, label string
		if entry.isDir {
			icon = "📁 "
			if entry.name == ".." {
				icon = "⬆  "
			}
			label = dirStyle.Render(entry.name + "/")
		} else {
			icon = "🖼  "
			label = fileStyle.Render(entry.name)
		}

		row := icon + label

		if i == fe.cursor {
			cursor := cursorStyle.Render("▶ ")
			row = cursor + row
		} else {
			row = "  " + row
		}

		sb.WriteString("  " + row)
		sb.WriteString("\n")
	}

	// Scroll indicator
	if total > listHeight {
		shown := fmt.Sprintf("%d–%d of %d", windowStart+1, windowEnd, total)
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  %s", shown)))
		sb.WriteString("\n")
	}

	sb.WriteString(dimStyle.Render(strings.Repeat("─", feModalWidth-2)))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  ↑/↓ navigate  ←/→ enter/exit dir  Enter select  Esc cancel"))

	return borderStyle.Render(sb.String())
}
