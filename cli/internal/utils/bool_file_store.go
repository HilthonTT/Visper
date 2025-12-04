package utils

import (
	"log/slog"
	"os"
	"strconv"
)

func ReadBoolFile(path string, defaultValue bool) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Error("Error in ReadBoolFile", "path", path, "error", err)
		return defaultValue
	}

	// Not using strconv.ParseBool() as it allows other values like : "TRUE"
	// Using exact string comparison with predefined constants ensures
	// consistent behavior and prevents issues with case-insensitivity or
	// unexpected values like "yes", "on", etc. that ParseBool would accept
	switch string(data) {
	case TrueString:
		return true
	case FalseString:
		return false
	default:
		return defaultValue
	}
}

func WriteBoolFile(path string, value bool) error {
	return os.WriteFile(path, []byte(strconv.FormatBool(value)), 0644)
}
