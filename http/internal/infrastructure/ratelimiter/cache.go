package ratelimiter

import (
	"errors"
	"time"
)

var ErrCacheMiss = errors.New("cache miss")

type GetterSetter interface {
	Get(key string) (int, error)
	Set(key string, value int) error
	SetWithExpiration(key string, value int, expiration time.Duration) error
	Close() error
}
