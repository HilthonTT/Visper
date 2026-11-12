package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

// Load builds the configuration from env vars, then args (flags), then the
// JSON file named by -config / PROXY_CONFIG if any.
func Load(args []string, getenv func(string) string) (*App, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	env := envReader{getenv: getenv}

	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	configPath := fs.String("config", env.str("PROXY_CONFIG", ""), "Path to JSON configuration file (env PROXY_CONFIG)")
	listenAddr := fs.String("listen", env.str("PROXY_LISTEN_ADDR", DefaultListenAddr), "Address to listen on (env PROXY_LISTEN_ADDR)")
	backends := fs.String("backends", env.str("PROXY_BACKENDS", DefaultBackendURL), "Comma-separated backend URLs (env PROXY_BACKENDS)")
	strategy := fs.String("strategy", env.str("PROXY_STRATEGY", DefaultStrategy), "Load balancing strategy (env PROXY_STRATEGY)")
	healthCheckInterval := fs.Duration("health-check-interval", env.duration("PROXY_HEALTH_CHECK_INTERVAL", DefaultHealthCheckInterval), "Health check interval (env PROXY_HEALTH_CHECK_INTERVAL)")
	healthCheckPath := fs.String("health-check-path", env.str("PROXY_HEALTH_CHECK_PATH", DefaultHealthCheckPath), "Backend health check path (env PROXY_HEALTH_CHECK_PATH)")
	maxFailCount := fs.Int("max-fail-count", env.int("PROXY_MAX_FAIL_COUNT", DefaultMaxFailCount), "Maximum failure count before marking backend as down (env PROXY_MAX_FAIL_COUNT)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	app := &App{
		ConfigPath: *configPath,
		LB: Config{
			ListenAddr:          *listenAddr,
			HealthCheckInterval: Duration(*healthCheckInterval),
			HealthCheckPath:     *healthCheckPath,
			MaxFailCount:        *maxFailCount,
			Strategy:            *strategy,
		},
		Server: Server{
			ReadHeaderTimeout: env.duration("PROXY_READ_HEADER_TIMEOUT", 10*time.Second),
			ReadTimeout:       env.duration("PROXY_READ_TIMEOUT", 60*time.Second),
			WriteTimeout:      env.duration("PROXY_WRITE_TIMEOUT", 60*time.Second),
			IdleTimeout:       env.duration("PROXY_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:   env.duration("PROXY_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Redis: Redis{
			SentinelAddrs:    splitList(env.str("REDIS_SENTINEL_ADDRS", "")),
			MasterName:       env.str("REDIS_MASTER_NAME", "mymaster"),
			Password:         env.str("REDIS_PASSWORD", ""),
			SentinelPassword: env.str("REDIS_SENTINEL_PASSWORD", ""),
			DialLocalhost:    env.bool("REDIS_DIAL_LOCALHOST", false),
		},
		Throttle: Throttle{
			GlobalMaxRequests: env.int("PROXY_THROTTLE_GLOBAL_MAX", 10_000),
			IPMaxRequests:     env.int("PROXY_THROTTLE_IP_MAX", 100),
			Interval:          env.duration("PROXY_THROTTLE_INTERVAL", time.Minute),
			Spans:             env.int("PROXY_THROTTLE_SPANS", 6),
			Cooldown:          env.duration("PROXY_THROTTLE_COOLDOWN", 30*time.Second),
		},
		TrustedProxies: splitList(env.str("PROXY_TRUSTED_PROXIES", "")),
		AdminToken:     env.str("PROXY_ADMIN_TOKEN", ""),
	}
	for _, u := range splitList(*backends) {
		app.LB.Backends = append(app.LB.Backends, BackendConfig{URL: u, Weight: 1})
	}

	if env.err != nil {
		return nil, env.err
	}

	if app.ConfigPath != "" {
		lb, err := LoadFile(app.ConfigPath)
		if err != nil {
			return nil, err
		}
		app.LB = *lb
	} else if err := app.LB.Validate(); err != nil {
		return nil, err
	}

	return app, nil
}

// LoadFile reads and validates a JSON load-balancer config file.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
