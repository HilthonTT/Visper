// Package env reads configuration values from environment variables.
package env

import "os"

// GetString returns the value of the environment variable key, or fallback
// if it is unset.
func GetString(key, fallback string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	return val
}
