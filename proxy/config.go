package main

import (
	"fmt"
	"time"
)

// Config represents the load balancer configuration
type Config struct {
	ListenAddr          string          `json:"listen_addr"`
	HealthCheckInterval time.Duration   `json:"health_check_interval"`
	MaxFailCount        int             `json:"max_fail_count"`
	Strategy            string          `json:"strategy"`
	Backends            []BackendConfig `json:"backends"`
}

// BackendConfig represents a backend server configuration
type BackendConfig struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

// parseStrategyString converts a strategy string to a Strategy enum
func parseStrategyString(s string) (Strategy, error) {
	switch s {
	case "round_robin":
		return RoundRobin, nil
	case "least_connections":
		return LeastConnections, nil
	case "ip_hash":
		return IPHash, nil
	case "random":
		return Random, nil
	case "weighted_round_robin":
		return WeightedRoundRobin, nil
	default:
		return 0, fmt.Errorf("unknown strategy: %s", s)
	}
}
