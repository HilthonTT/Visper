package resource

import "testing"

func TestAPIURL(t *testing.T) {
	cases := []struct {
		name           string
		visper, legacy string
		want           string
	}{
		{"default is the api, not the proxy", "", "", DefaultAPIURL},
		{"documented variable", "http://api:5005", "", "http://api:5005"},
		{"legacy variable still works", "", "http://old:1", "http://old:1"},
		{"documented variable wins", "http://new:2", "http://old:1", "http://new:2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VISPER_BASE_URL", tc.visper)
			t.Setenv("API_URL", tc.legacy)
			if got := apiURL(); got != tc.want {
				t.Fatalf("apiURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
