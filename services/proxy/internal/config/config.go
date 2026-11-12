// Package config loads the proxy configuration from (in increasing order of
// precedence) built-in defaults, environment variables, command-line flags
// and an optional JSON file for the load-balancer section.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"
)

// Defaults.
const (
	DefaultListenAddr          = ":5004"
	DefaultBackendURL          = "http://localhost:5005"
	DefaultStrategy            = "round_robin"
	DefaultHealthCheckInterval = 30 * time.Second
	DefaultHealthCheckPath     = "/health"
	DefaultMaxFailCount        = 3
)

// App is the complete runtime configuration.
type App struct {
	// ConfigPath is the optional JSON file overriding the LB section.
	ConfigPath     string
	LB             Config
	Server         Server
	Redis          Redis
	Throttle       Throttle
	TrustedProxies []string
	AdminToken     string

	// lbBase is the LB section from defaults, env and flags only; the JSON
	// file is overlaid on it both at startup and on every reload.
	lbBase Config
}

// Config is the load-balancer configuration. It is the part that can be
// supplied as a JSON file and reloaded at runtime.
type Config struct {
	ListenAddr          string          `json:"listen_addr"`
	HealthCheckInterval Duration        `json:"health_check_interval"`
	HealthCheckPath     string          `json:"health_check_path"`
	MaxFailCount        int             `json:"max_fail_count"`
	Strategy            string          `json:"strategy"`
	Backends            []BackendConfig `json:"backends"`
}

// BackendConfig is a single upstream server.
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
	seen := make(map[string]struct{}, len(c.Backends))
	for i := range c.Backends {
		b := &c.Backends[i]
		u, err := ParseBackendURL(b.URL)
		if err != nil {
			return err
		}
		key := u.String()
		if _, dup := seen[key]; dup {
			return fmt.Errorf("duplicate backend url %q", b.URL)
		}
		seen[key] = struct{}{}
		if b.Weight <= 0 {
			b.Weight = 1
		}
	}
	return nil
}

// ParseBackendURL parses and checks that s is an absolute http(s) URL.
func ParseBackendURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid backend url %q: %w", s, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid backend url %q: must be absolute http(s) URL", s)
	}
	return u, nil
}

func (c Config) clone() Config {
	c.Backends = slices.Clone(c.Backends)
	return c
}
