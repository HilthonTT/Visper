// Package apperr defines the application's error kinds. Use cases return
// *Error values carrying a Kind; the HTTP layer maps the kind to a status
// code, so handlers never have to inspect error messages.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error by what the caller did wrong (or didn't).
type Kind int

const (
	// Internal is a failure the caller can't fix; its message is not shown
	// to clients.
	Internal Kind = iota
	Invalid
	Unauthenticated
	Forbidden
	NotFound
	Gone
	TooLarge
)

// Error is an error of a given kind with a message safe to show to clients.
type Error struct {
	Kind    Kind
	Message string
	// Err is the underlying error, if any.
	Err error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Err }

// New returns an error of the given kind.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Newf returns an error of the given kind with a formatted message.
func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap gives err a kind, keeping its message.
func Wrap(kind Kind, err error) *Error {
	return &Error{Kind: kind, Message: err.Error(), Err: err}
}

// KindOf returns the kind of err, or Internal if it has none.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return Internal
}
