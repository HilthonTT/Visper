// Command visper is the terminal client for the Visper encrypted chat.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/cli/internal/tui"
)

func main() {
	logFile, err := openLogFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	}
	var logOut io.Writer = io.Discard //nolint:staticcheck // explicit interface type is needed for the reassignment below
	if logFile != nil {
		defer logFile.Close()
		logOut = logFile
	}
	// slog.SetDefault also routes the standard library "log" package through
	// this handler, so nothing is written to the terminal behind the TUI.
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, nil)))

	model, err := tui.NewModel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initialising:", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error running program:", err)
		os.Exit(1)
	}
}

// openLogFile creates (truncating) visper.log in the user's cache directory,
// e.g. ~/.cache/visper on Linux, ~/Library/Caches/visper on macOS and
// %LocalAppData%\visper on Windows.
func openLogFile() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "visper")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "visper.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}
