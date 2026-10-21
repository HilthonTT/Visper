package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hilthontt/visper/proxy/internal/balancer"
	"github.com/hilthontt/visper/proxy/internal/config"
	"github.com/hilthontt/visper/proxy/internal/metrics"
	"github.com/hilthontt/visper/proxy/internal/middleware"
	"github.com/hilthontt/visper/proxy/internal/throttling"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		return err
	}

	strategy, err := balancer.ParseStrategy(cfg.LB.Strategy)
	if err != nil {
		return err
	}

	ipResolver, err := middleware.NewClientIPResolver(cfg.TrustedProxies)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lb, err := balancer.New(cfg.LB.Backends, balancer.Options{
		Strategy:            strategy,
		HealthCheckInterval: time.Duration(cfg.LB.HealthCheckInterval),
		HealthCheckPath:     cfg.LB.HealthCheckPath,
		MaxFailCount:        cfg.LB.MaxFailCount,
		Metrics:             metrics.New("loadBalancer", prometheus.DefaultRegisterer),
		ClientIP:            ipResolver.ClientIP,
		TrustForwarded:      ipResolver.TrustsPeer,
	})
	if err != nil {
		return err
	}
	go lb.RunHealthChecks(ctx)

	redisClient := newRedisClient(cfg.Redis)
	if redisClient != nil {
		defer redisClient.Close()
	} else {
		log.Printf("REDIS_SENTINEL_ADDRS not set: throttling is local to this process")
	}

	t := cfg.Throttle
	ht := throttling.NewHierarchicalThrottler(
		// Most specific level first so a blocked client does not burn global quota.
		&throttling.ThrottleLevel{
			Name:         "per-ip",
			KeyExtractor: ipResolver.ClientIP,
			Throttler: throttling.NewThrottler(throttling.ThrottleConfig{
				Name: "ip", MaxRequests: t.IPMaxRequests, Interval: t.Interval, Spans: t.Spans, Cooldown: t.Cooldown,
			}, redisClient),
		},
		&throttling.ThrottleLevel{
			Name:         "global",
			KeyExtractor: func(r *http.Request) string { return "global" },
			Throttler: throttling.NewThrottler(throttling.ThrottleConfig{
				Name: "global", MaxRequests: t.GlobalMaxRequests, Interval: t.Interval, Spans: t.Spans, Cooldown: t.Cooldown,
			}, redisClient),
		},
	)
	throttleDone := make(chan struct{})
	go func() {
		defer close(throttleDone)
		if err := ht.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("throttler stopped: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/", lb)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("POST /admin/reload", middleware.RequireBearerToken(cfg.AdminToken, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if cfg.ConfigPath == "" {
				http.Error(w, "no config file provided", http.StatusConflict)
				return
			}
			lbCfg, err := config.LoadFile(cfg.ConfigPath)
			if err == nil {
				err = lb.Reload(lbCfg)
			}
			if err != nil {
				log.Printf("config reload failed: %v", err)
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Configuration reloaded successfully"))
		})))

	server := &http.Server{
		Addr:              cfg.LB.ListenAddr,
		Handler:           middleware.Chain(mux, middleware.HierarchicalThrottling(ht)),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	errc := make(chan error, 1)
	go func() {
		log.Printf("Starting load balancer on %s with strategy: %s", cfg.LB.ListenAddr, cfg.LB.Strategy)
		log.Printf("Metrics available at %s/metrics", cfg.LB.ListenAddr)
		errc <- server.ListenAndServe()
	}()

	select {
	case err := <-errc:
		stop()
		<-throttleDone
		return err
	case <-ctx.Done():
	}

	log.Printf("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	<-throttleDone
	return err
}

// newRedisClient returns a nil interface (not a typed nil) when Redis is not configured.
func newRedisClient(c config.Redis) redis.UniversalClient {
	if len(c.SentinelAddrs) == 0 {
		return nil
	}
	opts := &redis.FailoverOptions{
		MasterName:       c.MasterName,
		SentinelAddrs:    c.SentinelAddrs,
		Password:         c.Password,
		SentinelPassword: c.SentinelPassword,
	}
	if c.DialLocalhost {
		// Sentinel reports internal Docker IPs (172.18.x.x) for the master.
		// Redirect them to localhost since ports are mapped to the host.
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp4", "127.0.0.1:"+port)
		}
	}
	return redis.NewFailoverClient(opts)
}
