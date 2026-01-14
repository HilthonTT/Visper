package sign

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrSignExpired   = errors.New("sign expired")
	ErrSignInvalid   = errors.New("sign invalid")
	ErrExpireInvalid = errors.New("expire invalid")
	ErrExpireMissing = errors.New("expire missing")
)

type ISign interface {
	Sign(data string, expire int64) string
	Verify(data, sign string) error
}

var once sync.Once
var instance ISign

func Sign(data string) string {
	expire := 0 // TODO: Add it to the config
	if expire == 0 {
		return NotExpired(data)
	} else {
		return WithDuration(data, time.Duration(expire)*time.Hour)
	}
}

func WithDuration(data string, d time.Duration) string {
	once.Do(Instance)
	return instance.Sign(data, time.Now().Add(d).Unix())
}

func NotExpired(data string) string {
	once.Do(Instance)
	return instance.Sign(data, 0)
}

func Verify(data string, sign string) error {
	once.Do(Instance)
	return instance.Verify(data, sign)
}

func Instance() {
	// TODO: Use config's Token
	instance = NewHMACSign([]byte("TOKEN"))
}
