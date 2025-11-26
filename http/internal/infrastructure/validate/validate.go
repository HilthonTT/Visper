// package validate
package validate

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
)

// Validator is a function that validates a string and returns an error if invalid
type Validator func(value string) error

// Field creates a labeled validator with a custom name for better error messages
func Field(name string, validators ...Validator) Validator {
	return func(value string) error {
		for _, v := range validators {
			if err := v(value); err != nil {
				// Rewrite error to include field name if not already present
				if !strings.Contains(err.Error(), name) {
					return fmt.Errorf("%s: %w", name, err)
				}
				return err
			}
		}
		return nil
	}
}

// Compose chains multiple validators — first error wins
func Compose(validators ...Validator) Validator {
	return func(value string) error {
		for _, v := range validators {
			if err := v(value); err != nil {
				return err
			}
		}
		return nil
	}
}

// Required ensures the field is not empty
func Required() Validator {
	return func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("this field is required")
		}
		return nil
	}
}

// MinLength checks minimum length
func MinLength(min int) Validator {
	return func(v string) error {
		if len(v) < min {
			return fmt.Errorf("must be at least %d characters", min)
		}
		return nil
	}
}

// MaxLength checks maximum length
func MaxLength(max int) Validator {
	return func(v string) error {
		if len(v) > max {
			return fmt.Errorf("must be no more than %d characters", max)
		}
		return nil
	}
}

// Length checks exact length
func Length(exact int) Validator {
	return func(v string) error {
		if len(v) != exact {
			return fmt.Errorf("must be exactly %d characters", exact)
		}
		return nil
	}
}

// LengthBetween checks length between min and max (inclusive)
func LengthBetween(min, max int) Validator {
	return Compose(MinLength(min), MaxLength(max))
}

// DigitsOnly ensures string contains only digits
func DigitsOnly() Validator {
	return func(v string) error {
		if v == "" {
			return nil // let Required handle empty
		}
		for _, c := range v {
			if !unicode.IsDigit(c) {
				return fmt.Errorf("must contain only digits")
			}
		}
		return nil
	}
}

// Email validates email format using net/mail + common sense
func Email() Validator {
	return func(v string) error {
		if v == "" {
			return nil
		}
		_, err := mail.ParseAddress(v)
		if err != nil {
			return fmt.Errorf("must be a valid email address")
		}
		return nil
	}
}

// Regex matches a regular expression
func Regex(pattern string) Validator {
	re := regexp.MustCompile(pattern)
	return func(v string) error {
		if !re.MatchString(v) {
			return fmt.Errorf("invalid format")
		}
		return nil
	}
}

// Matches checks if value matches a regex (alias with custom message)
func Matches(pattern, message string) Validator {
	re := regexp.MustCompile(pattern)
	return func(v string) error {
		if !re.MatchString(v) {
			if message != "" {
				return fmt.Errorf("%s", message)
			}
			return fmt.Errorf("invalid format")
		}
		return nil
	}
}

// Luhn validates credit card numbers using the Luhn algorithm
func Luhn() Validator {
	return func(v string) error {
		// Clean non-digits
		clean := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, v)

		if len(clean) < 13 || len(clean) > 19 {
			return fmt.Errorf("invalid credit card number length")
		}

		sum := 0
		double := false
		for i := len(clean) - 1; i >= 0; i-- {
			digit := int(clean[i] - '0')
			if double {
				digit *= 2
				if digit > 9 {
					digit -= 9
				}
			}
			sum += digit
			double = !double
		}

		if sum%10 != 0 {
			return fmt.Errorf("invalid credit card number")
		}
		return nil
	}
}

// OneOf checks if value is in allowed list
func OneOf(allowed ...string) Validator {
	set := make(map[string]bool)
	for _, a := range allowed {
		set[a] = true
	}
	return func(v string) error {
		if !set[v] {
			return fmt.Errorf("must be one of: %s", strings.Join(allowed, ", "))
		}
		return nil
	}
}

// NoSpaces disallows spaces
func NoSpaces() Validator {
	return Matches(`^\S+$`, "must not contain spaces")
}

// Alphanumeric only letters and numbers
func Alphanumeric() Validator {
	return Matches(`^[a-zA-Z0-9]+$`, "must contain only letters and numbers")
}

// Lowercase enforces lowercase
func Lowercase() Validator {
	return func(v string) error {
		if v != strings.ToLower(v) {
			return fmt.Errorf("must be lowercase")
		}
		return nil
	}
}

// Uppercase enforces uppercase
func Uppercase() Validator {
	return func(v string) error {
		if v != strings.ToUpper(v) {
			return fmt.Errorf("must be uppercase")
		}
		return nil
	}
}
