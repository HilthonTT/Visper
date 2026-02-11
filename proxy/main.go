package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Define command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	listenAddr := flag.String("listen", ":5004", "Address to listen on")
	strategyStr := flag.String("strategy", "round_robin", "Load balancing strategy")
	healthCheckInterval := flag.Duration("health-check-interval", 30*time.Second, "Health check interval")
	maxFailCount := flag.Int("max-fail-count", 3, "Maximum failure count before marking backend as down")

	flag.Parse()

	var config Config

	// If config file is provided, load it
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatalf("Error reading config file: %v", err)
		}

		if err := json.Unmarshal(data, &config); err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	} else {
		// Use command line flags
		config = Config{
			ListenAddr:          *listenAddr,
			HealthCheckInterval: *healthCheckInterval,
			MaxFailCount:        *maxFailCount,
			Strategy:            *strategyStr,
			Backends: []BackendConfig{
				{URL: "http://localhost:5005", Weight: 1},
			},
		}
	}

	// Parse strategy
	strategy, err := parseStrategyString(config.Strategy)
	if err != nil {
		log.Fatalf("Invalid strategy: %v", err)
	}

	// Extract backends and weights
	backendURLs := make([]string, len(config.Backends))
	weights := make([]int, len(config.Backends))

	for i, backend := range config.Backends {
		backendURLs[i] = backend.URL
		weights[i] = backend.Weight
	}

	metrics := NewMetrics("loadBalancer")

	// Create load balancer
	lb := NewLoadBalancer(
		backendURLs,
		weights,
		config.HealthCheckInterval,
		config.MaxFailCount,
		strategy,
	)
	lb.metrics = metrics

	mux := http.NewServeMux()
	mux.Handle("/", lb)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/admin/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Reload configuration
		if err := reloadConfiguration(lb, *configPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Configuration reloaded successfully"))
	})

	// Start server
	server := http.Server{
		Addr:    config.ListenAddr,
		Handler: mux,
	}

	log.Printf("Starting load balancer on %s with strategy: %s", config.ListenAddr, config.Strategy)
	log.Printf("Metrics available at %s/metrics", config.ListenAddr)
	log.Fatal(server.ListenAndServe())
}

func reloadConfiguration(lb *LoadBalancer, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("no config file provided")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	strategy, err := parseStrategyString(config.Strategy)
	if err != nil {
		return fmt.Errorf("invalid strategy: %v", err)
	}

	backendURLs := make([]string, len(config.Backends))
	weights := make([]int, len(config.Backends))

	// Extract backends and weights
	for i, backend := range config.Backends {
		backendURLs[i] = backend.URL
		weights[i] = backend.Weight
	}

	// Update load balancer configuration
	lb.mux.Lock()
	lb.healthCheckInterval = config.HealthCheckInterval
	lb.maxFailCount = config.MaxFailCount
	lb.strategy = strategy

	// Update backends (keep the existing ones if they're still in the config)
	oldBackends := lb.backends
	lb.backends = make([]*Backend, len(config.Backends))

	for i, backendURL := range backendURLs {
		// Check if this backend already exists
		found := false
		for _, oldBackend := range oldBackends {
			// Keep the existing backend but update its weight
			lb.backends[i] = oldBackend
			oldBackend.weight = weights[i]
			found = true
			break
		}

		if !found {
			parsedURL, _ := url.Parse(backendURL) // Error already checked earlier
			lb.backends[i] = &Backend{
				URL:          parsedURL,
				Alive:        true, // Assume alive until health check
				ReverseProxy: createOptimizedReverseProxy(parsedURL),
				weight:       weights[i],
			}
		}
	}

	lb.mux.Unlock()

	log.Printf("Configuration reloaded with %d backends and strategy: %s", len(lb.backends), config.Strategy)
	return nil
}
