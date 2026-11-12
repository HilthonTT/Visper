package config

import (
	"encoding/json"
	"fmt"
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
