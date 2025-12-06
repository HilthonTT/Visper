package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hilthontt/visper/cli/internal/tui"
)

func main() {
	log, err := os.Create("output.log")
	if err != nil {
		panic(err)
	}
	defer log.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{})))

	model, err := tui.NewModel()
	if err != nil {
		panic(err)
	}

	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
