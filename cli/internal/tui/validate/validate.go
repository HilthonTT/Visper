// Package validate provides small composable string validators.
package validate

import (
	"fmt"
	"unicode/utf8"
)

// ErrorHandler validates str and returns a descriptive error if it is invalid.
type ErrorHandler func(str string) error

// WithinLen requires str to be between min and max characters (runes)
// long, inclusive.
func WithinLen(min, max int, name string) ErrorHandler {
	return func(str string) error {
		n := utf8.RuneCountInString(str)
		if n >= min && n <= max {
			return nil
		}

		return fmt.Errorf(
			"expected %s to be between %d and %d characters, but got %d",
			name,
			min,
			max,
			n,
		)
	}
}

// MaxBytes requires str to be at most max bytes long.
func MaxBytes(max int, name string) ErrorHandler {
	return func(str string) error {
		if len(str) <= max {
			return nil
		}
		return fmt.Errorf("expected %s to be at most %d bytes, but got %d", name, max, len(str))
	}
}

// NotEmpty requires str to be non-empty.
func NotEmpty(name string) ErrorHandler {
	return func(str string) error {
		if len(str) == 0 {
			return fmt.Errorf("%s cannot be empty", name)
		}
		return nil
	}
}

// Compose runs the validators in order and returns the first error.
func Compose(input ...ErrorHandler) ErrorHandler {
	return func(str string) error {
		for _, f := range input {
			err := f(str)
			if err != nil {
				return err
			}
		}
		return nil
	}
}
