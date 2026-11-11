package repository

import "errors"

// ErrNotFound is returned by repositories when the requested record does not
// exist. Any other error is a storage failure.
var ErrNotFound = errors.New("record not found")
