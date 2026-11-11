package config

import (
	"reflect"
	"testing"
)

func TestTrustedProxyList(t *testing.T) {
	cases := map[string][]string{
		"":                          nil,
		" , ":                       nil,
		"10.0.0.5":                  {"10.0.0.5"},
		" 10.0.0.5 , 172.16.0.0/12": {"10.0.0.5", "172.16.0.0/12"},
	}
	for in, want := range cases {
		if got := (ServerConfig{TrustedProxies: in}).TrustedProxyList(); !reflect.DeepEqual(got, want) {
			t.Errorf("TrustedProxyList(%q) = %v, want %v", in, got, want)
		}
	}
}
