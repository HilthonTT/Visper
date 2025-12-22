package health

// healthResponse represents the health status of the API
type healthResponse struct {
	Status    string `json:"status" example:"ok" enum:"ok,unhealthy"`  // Health status (ok or unhealthy)
	Timestamp string `json:"timestamp" example:"2024-01-01T12:00:00Z"` // Current server timestamp in RFC3339 format
	Uptime    string `json:"uptime" example:"2h30m45s"`                // Server uptime since start
}
