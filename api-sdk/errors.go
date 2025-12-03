package apisdk

import "errors"

var (
	ErrMissingIDParameter       = errors.New("missing required id parameter")
	ErrMissingJoinCodeParameter = errors.New("missing required join code parameter")
	ErrMissingUsername          = errors.New("missing required username parameter")
)
