package validate

import "fmt"

type ErrorHandler func(str string) error

func WithinLen(min, max int, name string) ErrorHandler {
	return func(str string) error {
		if len(str) >= min && len(str) <= max {
			return nil
		}

		return fmt.Errorf(
			"expected %s to be between %d and %d, but got %d",
			name,
			min,
			max,
			len(str),
		)
	}
}

func MustBeLen(length int, name string) ErrorHandler {
	return func(str string) error {
		if str == "" {
			return nil
		}
		if len(str) != length {
			return fmt.Errorf("Expected %s to be length %d but got %d", name, length, len(str))
		}
		return nil
	}
}

func NotEmpty(name string) ErrorHandler {
	return func(str string) error {
		if len(str) == 0 {
			return fmt.Errorf("%s cannot empty", name)
		}
		return nil
	}
}

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
