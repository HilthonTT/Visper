package utils

import (
	"fmt"
	"log/slog"
	"os"
)

func PrintlnAndExit(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func PrintfAndExitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func SetRootLoggerToStdout(debug bool) {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func SetRootLoggerToDiscarded() {
	slog.SetDefault(slog.New(slog.DiscardHandler))
}
