package validate

import (
	"strings"
	"testing"
)

func TestValidators(t *testing.T) {
	tests := []struct {
		name    string
		v       ErrorHandler
		in      string
		wantErr bool
	}{
		{"not empty ok", NotEmpty("x"), "a", false},
		{"not empty fails", NotEmpty("x"), "", true},
		{"within len ok", WithinLen(1, 3, "x"), "abc", false},
		{"within len too short", WithinLen(1, 3, "x"), "", true},
		{"within len too long", WithinLen(1, 3, "x"), "abcd", true},
		{"within len counts runes", WithinLen(1, 3, "x"), "世界!", false},
		{"max bytes ok", MaxBytes(3, "x"), "abc", false},
		{"max bytes counts bytes", MaxBytes(3, "x"), "世", false},
		{"max bytes fails", MaxBytes(3, "x"), "世!", true},
		{"compose ok", Compose(NotEmpty("x"), WithinLen(1, 5, "x")), "hey", false},
		{"compose first fails", Compose(NotEmpty("x"), WithinLen(1, 5, "x")), "", true},
		{"compose second fails", Compose(NotEmpty("x"), WithinLen(1, 5, "x")), strings.Repeat("a", 6), true},
		{"compose empty", Compose(), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.v(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
