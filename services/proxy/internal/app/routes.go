package app

import (
	"log"
	"net/http"

	"github.com/hilthontt/visper/services/proxy/internal/balancer"
	"github.com/hilthontt/visper/services/proxy/internal/config"
	"github.com/hilthontt/visper/services/proxy/internal/middleware"
	"github.com/hilthontt/visper/services/proxy/internal/throttling"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// routes builds the top-level handler. Every route is throttled.
func routes(cfg *config.App, lb *balancer.LoadBalancer, throttle *throttling.Hierarchy) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", lb)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("POST /admin/reload", middleware.Chain(reloadHandler(cfg, lb),
		middleware.RequireBearerToken(cfg.AdminToken)))
	return middleware.Chain(mux, middleware.Throttle(throttle))
}

// reloadHandler re-reads the config file (overlaid on the env/flag settings)
// and applies it to the load balancer.
func reloadHandler(cfg *config.App, lb *balancer.LoadBalancer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ConfigPath == "" {
			http.Error(w, "no config file provided", http.StatusConflict)
			return
		}
		lbCfg, err := cfg.LoadLB()
		if err == nil {
			err = lb.Reload(lbCfg)
		}
		if err != nil {
			log.Printf("config reload failed: %v", err)
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if lbCfg.ListenAddr != cfg.LB.ListenAddr {
			log.Printf("config reload: listen_addr change to %s requires a restart", lbCfg.ListenAddr)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Configuration reloaded successfully\n"))
	}
}
