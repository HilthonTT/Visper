package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	log, err := os.Create("output.log")
	if err != nil {
		panic(err)
	}
	defer log.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{})))

	fmt.Println("Hello World")
}
