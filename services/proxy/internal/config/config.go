// Package config loads the proxy configuration from (in increasing order of
// precedence) built-in defaults, environment variables, command-line flags
// and an optional JSON config file for the load-balancer section.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration that unmarshals from JSON strings such as
// "30s" as well as from integer nanoseconds.
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch val := v.(type) {
	case float64:
		*d = Duration(time.Duration(val))
	case string:
		parsed, err := time.ParseDuration(val)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
	default:
		return fmt.Errorf("invalid duration %s", b)
	}
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Config represents the load balancer configuration
type Config struct {
	ListenAddr          string          `json:"listen_addr"`
	HealthCheckInterval Duration        `json:"health_check_interval"`
	HealthCheckPath     string          `json:"health_check_path"`
	MaxFailCount        int             `json:"max_fail_count"`
	Strategy            string          `json:"strategy"`
	Backends            []BackendConfig `json:"backends"`
}

// BackendConfig represents a backend server configuration
type BackendConfig struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

// Server holds HTTP server settings.
type Server struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Redis holds the optional Redis (Sentinel) settings used for distributed
// throttling. When SentinelAddrs is empty throttling is local to the process.
type Redis struct {
	SentinelAddrs    []string
	MasterName       string
	Password         string
	SentinelPassword string
	// DialLocalhost rewrites every Redis address to 127.0.0.1:<port>. Needed
	// when Sentinel reports internal Docker IPs for a master whose ports are
	// mapped to the host (local development only).
	DialLocalhost bool
}

// Throttle holds the per-IP and global rate limits.
type Throttle struct {
	GlobalMaxRequests int
	IPMaxRequests     int
	Interval          time.Duration
	Spans             int
	Cooldown          time.Duration
}

// App is the complete runtime configuration.
type App struct {
	ConfigPath     string
	LB             Config
	Server         Server
	Redis          Redis
	Throttle       Throttle
	TrustedProxies []string
	AdminToken     string
}

// Defaults
const (
	DefaultListenAddr          = ":5004"
	DefaultBackendURL          = "http://localhost:5005"
	DefaultStrategy            = "round_robin"
	DefaultHealthCheckInterval = 30 * time.Second
	DefaultHealthCheckPath     = "/health"
	DefaultMaxFailCount        = 3
)

// Load builds the configuration from env vars, then args (flags), then the
// JSON file named by -config / PROXY_CONFIG if any.
func Load(args []string, getenv func(string) string) (*App, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	env := envReader{getenv: getenv}

	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	configPath := fs.String("config", env.strVal("PROXY_CONFIG", ""), "Path to JSON configuration file (env PROXY_CONFIG)")
	listenAddr := fs.String("listen", env.strVal("PROXY_LISTEN_ADDR", DefaultListenAddr), "Address to listen on (env PROXY_LISTEN_ADDR)")
	backends := fs.String("backends", env.strVal("PROXY_BACKENDS", DefaultBackendURL), "Comma-separated backend URLs (env PROXY_BACKENDS)")
	strategy := fs.String("strategy", env.strVal("PROXY_STRATEGY", DefaultStrategy), "Load balancing strategy (env PROXY_STRATEGY)")
	healthCheckInterval := fs.Duration("health-check-interval", env.durVal("PROXY_HEALTH_CHECK_INTERVAL", DefaultHealthCheckInterval), "Health check interval (env PROXY_HEALTH_CHECK_INTERVAL)")
	healthCheckPath := fs.String("health-check-path", env.strVal("PROXY_HEALTH_CHECK_PATH", DefaultHealthCheckPath), "Backend health check path (env PROXY_HEALTH_CHECK_PATH)")
	maxFailCount := fs.Int("max-fail-count", env.intVal("PROXY_MAX_FAIL_COUNT", DefaultMaxFailCount), "Maximum failure count before marking backend as down (env PROXY_MAX_FAIL_COUNT)")
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
			ReadHeaderTimeout: env.durVal("PROXY_READ_HEADER_TIMEOUT", 10*time.Second),
			ReadTimeout:       env.durVal("PROXY_READ_TIMEOUT", 60*time.Second),
			WriteTimeout:      env.durVal("PROXY_WRITE_TIMEOUT", 60*time.Second),
			IdleTimeout:       env.durVal("PROXY_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:   env.durVal("PROXY_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Redis: Redis{
			SentinelAddrs:    splitList(env.strVal("REDIS_SENTINEL_ADDRS", "")),
			MasterName:       env.strVal("REDIS_MASTER_NAME", "mymaster"),
			Password:         env.strVal("REDIS_PASSWORD", ""),
			SentinelPassword: env.strVal("REDIS_SENTINEL_PASSWORD", ""),
			DialLocalhost:    env.boolVal("REDIS_DIAL_LOCALHOST", false),
		},
		Throttle: Throttle{
			GlobalMaxRequests: env.intVal("PROXY_THROTTLE_GLOBAL_MAX", 10_000),
			IPMaxRequests:     env.intVal("PROXY_THROTTLE_IP_MAX", 100),
			Interval:          env.durVal("PROXY_THROTTLE_INTERVAL", time.Minute),
			Spans:             env.intVal("PROXY_THROTTLE_SPANS", 6),
			Cooldown:          env.durVal("PROXY_THROTTLE_COOLDOWN", 30*time.Second),
		},
		TrustedProxies: splitList(env.strVal("PROXY_TRUSTED_PROXIES", "")),
		AdminToken:     env.strVal("PROXY_ADMIN_TOKEN", ""),
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

// Validate applies defaults to zero values and checks the configuration.
func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		c.ListenAddr = DefaultListenAddr
	}
	if c.Strategy == "" {
		c.Strategy = DefaultStrategy
	}
	if c.HealthCheckInterval <= 0 {
		c.HealthCheckInterval = Duration(DefaultHealthCheckInterval)
	}
	if c.HealthCheckPath == "" {
		c.HealthCheckPath = DefaultHealthCheckPath
	}
	if c.MaxFailCount <= 0 {
		c.MaxFailCount = DefaultMaxFailCount
	}
	if len(c.Backends) == 0 {
		return errors.New("at least one backend is required")
	}
	for i := range c.Backends {
		b := &c.Backends[i]
		u, err := url.Parse(b.URL)
		if err != nil {
			return fmt.Errorf("invalid backend url %q: %w", b.URL, err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid backend url %q: must be absolute http(s) URL", b.URL)
		}
		if b.Weight <= 0 {
			b.Weight = 1
		}
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type envReader struct {
	getenv func(string) string
	err    error
}

func (e *envReader) strVal(key, def string) string {
	if v := strings.TrimSpace(e.getenv(key)); v != "" {
		return v
	}
	return def
}

func (e *envReader) intVal(key string, def int) int {
	v := e.strVal(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.err = errors.Join(e.err, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return n
}

func (e *envReader) durVal(key string, def time.Duration) time.Duration {
	v := e.strVal(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.err = errors.Join(e.err, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return d
}

func (e *envReader) boolVal(key string, def bool) bool {
	v := e.strVal(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.err = errors.Join(e.err, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return b
}
