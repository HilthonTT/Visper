package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// envReader reads typed environment values, collecting parse errors so they
// can all be reported at once.
type envReader struct {
	getenv func(string) string
	err    error
}

func (e *envReader) str(key, def string) string {
	if v := strings.TrimSpace(e.getenv(key)); v != "" {
		return v
	}
	return def
}

func (e *envReader) int(key string, def int) int {
	return parse(e, key, def, strconv.Atoi)
}

func (e *envReader) duration(key string, def time.Duration) time.Duration {
	return parse(e, key, def, time.ParseDuration)
}

func (e *envReader) bool(key string, def bool) bool {
	return parse(e, key, def, strconv.ParseBool)
}

func parse[T any](e *envReader, key string, def T, fn func(string) (T, error)) T {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	out, err := fn(v)
	if err != nil {
		e.err = errors.Join(e.err, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return out
}

// splitList splits a comma-separated list, dropping empty entries.
func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
