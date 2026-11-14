// Command visper is the terminal client for the Visper encrypted chat.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
	"github.com/hilthontt/visper/clients/cli/internal/tui"
)

// version is set at release time with -ldflags "-X main.version=…".
var version = ""

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()
	if *showVersion {
		fmt.Println("visper", buildVersion())
		return
	}

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

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, `visper %s: anonymous, end-to-end encrypted chat rooms in your terminal.

Usage: visper [-version]

Environment:
  VISPER_BASE_URL  URL of the Visper API or proxy (default http://localhost:5005)

`, buildVersion())
	flag.PrintDefaults()
}

// buildVersion is the release version, else the module version recorded by
// "go install", else "dev".
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
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
