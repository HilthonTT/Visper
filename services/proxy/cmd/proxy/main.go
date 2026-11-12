// Command proxy runs the Visper load balancer.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hilthontt/visper/services/proxy/internal/app"
	"github.com/hilthontt/visper/services/proxy/internal/config"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
