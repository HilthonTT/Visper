// Package app wires the proxy's components together and runs the HTTP server.
package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"github.com/hilthontt/visper/services/proxy/internal/balancer"
	"github.com/hilthontt/visper/services/proxy/internal/clientip"
	"github.com/hilthontt/visper/services/proxy/internal/config"
	"github.com/hilthontt/visper/services/proxy/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Run starts the proxy and blocks until ctx is cancelled (then shuts down
// gracefully) or the server fails.
func Run(ctx context.Context, cfg *config.App) error {
	ips, err := clientip.NewResolver(cfg.TrustedProxies)
	if err != nil {
		return err
	}

	lb, err := balancer.New(&cfg.LB, balancer.Options{
		Metrics:        metrics.New("loadBalancer", prometheus.DefaultRegisterer),
		ClientIP:       ips.ClientIP,
		TrustForwarded: ips.TrustsPeer,
	})
	if err != nil {
		return err
	}

	rdb := newRedisClient(cfg.Redis)
	if rdb != nil {
		defer rdb.Close()
	} else {
		log.Printf("REDIS_SENTINEL_ADDRS not set: throttling is local to this process")
	}
	throttle := newThrottle(cfg.Throttle, ips, rdb)

	server := &http.Server{
		Addr:              cfg.LB.ListenAddr,
		Handler:           routes(cfg, lb, throttle),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	// Background workers stop when bgCtx is cancelled, after the server.
	bgCtx, stopBg := context.WithCancel(context.Background())
	var bg sync.WaitGroup
	defer func() {
		stopBg()
		bg.Wait()
	}()
	bg.Go(func() { lb.RunHealthChecks(bgCtx) })
	bg.Go(func() {
		if err := throttle.Start(bgCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("throttler stopped: %v", err)
		}
	})

	errc := make(chan error, 1)
	go func() {
		log.Printf("load balancer listening on %s (strategy %s, %d backends)",
			cfg.LB.ListenAddr, cfg.LB.Strategy, len(cfg.LB.Backends))
		errc <- server.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Printf("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
