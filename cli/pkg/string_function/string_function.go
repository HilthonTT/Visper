package stringfunction

import (
	"fmt"
	"math"
)

// FormatFileSize converts a file size in bytes (like 5242880) into a nice human-readable string
// like "5.00 MB" or "1.37 GB".
//
// It uses decimal/SI units (1000-based) rather than binary units (1024-based).
//
// Example outputs:
//
//	0               → "0B"
//	512             → "512.00 B"
//	1536            → "1.54 kB"
//	2_500_000       → "2.50 MB"
//	7_429_000_000   → "7.43 GB"
func FormatFileSize(size int64) string {
	// Special case: if size is exactly zero, just return "0B"
	// This prevents showing "0.00 B" and avoids log(0) problems
	if size == 0 {
		return "0B"
	}

	// List of unit names in order (B → kB → MB → GB → etc.)
	// We're using decimal (SI) prefixes: 1 kB = 1000 bytes, 1 MB = 1000 kB, etc.
	unitsDec := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

	// This math finds out which unit to use (0 = bytes, 1 = kilobytes, 2 = megabytes, etc.)
	//
	// Steps explained:
	// 1. math.Log(float64(size))        → natural log of the size
	// 2. math.Log(1000)                 → natural log of 1000
	// 3. log(size) / log(1000)          → = log base 1000 of size
	//    (this tells us the "power" we're dealing with)
	// 4. math.Floor(...)                → round down to get whole number
	// 5. int(...)                       → convert to integer to use as array index
	//
	// Examples:
	//   900 bytes     → log₁₀₀₀ ≈ 0.95  → Floor → 0 → "B"
	//   2_000 bytes   → log₁₀₀₀ ≈ 1.30  → Floor → 1 → "kB"
	//   5_000_000     → log₁₀₀₀ ≈ 2.70  → Floor → 2 → "MB"
	unitIndex := int(math.Floor(math.Log(float64(size)) / math.Log(1000)))

	// Calculate how many times we need to divide by 1000 to get the number
	// in the chosen unit
	//
	// Example:
	//   size = 2_560_000 bytes
	//   unitIndex = 2 (MB)
	//   1000² = 1_000_000
	//   2_560_000 / 1_000_000 = 2.56
	adjustedSize := float64(size) / math.Pow(1000, float64(unitIndex))

	// Format the number with 2 decimal places, add space, then add the unit
	// %.2f means: floating-point number with exactly 2 digits after decimal
	return fmt.Sprintf("%.2f %s", adjustedSize, unitsDec[unitIndex])
}
