package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
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

	// Create load balancer
	lb := NewLoadBalancer(
		backendURLs,
		weights,
		config.HealthCheckInterval,
		config.MaxFailCount,
		strategy,
	)

	// Start server
	server := http.Server{
		Addr:    config.ListenAddr,
		Handler: lb,
	}

	log.Printf("Starting load balancer on %s with strategy: %s", config.ListenAddr, config.Strategy)
	log.Fatal(server.ListenAndServe())
}
