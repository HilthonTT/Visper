package apijson

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hilthontt/visper/api-sdk/internal/param"
)

type sample struct {
	Name    string    `json:"name"`
	Count   int       `json:"count"`
	Enabled bool      `json:"enabled"`
	When    time.Time `json:"when"`
	Tags    []string  `json:"tags"`
}

func TestUnmarshalRoot(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want sample
	}{
		{
			name: "all fields",
			in:   `{"name":"a","count":3,"enabled":true,"when":"2026-01-02T03:04:05Z","tags":["x","y"]}`,
			want: sample{Name: "a", Count: 3, Enabled: true, When: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Tags: []string{"x", "y"}},
		},
		{
			name: "missing fields stay zero",
			in:   `{"name":"b"}`,
			want: sample{Name: "b"},
		},
		{
			name: "loose number from string",
			in:   `{"count":"7"}`,
			want: sample{Count: 7},
		},
		{
			name: "unknown fields ignored",
			in:   `{"name":"c","extra":1}`,
			want: sample{Name: "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got sample
			if err := UnmarshalRoot([]byte(tt.in), &got); err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.want.Name || got.Count != tt.want.Count || got.Enabled != tt.want.Enabled ||
				!got.When.Equal(tt.want.When) || len(got.Tags) != len(tt.want.Tags) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got.Tags {
				if got.Tags[i] != tt.want.Tags[i] {
					t.Fatalf("tags = %v, want %v", got.Tags, tt.want.Tags)
				}
			}
		})
	}
}

func TestUnmarshalRequiresPointer(t *testing.T) {
	if err := Unmarshal([]byte(`{}`), sample{}); err == nil {
		t.Fatal("expected error for non-pointer destination")
	}
}

type params struct {
	Name  param.Field[string] `json:"name"`
	Count param.Field[int64]  `json:"count"`
	Note  param.Field[string] `json:"note"`
}

func TestMarshalParamFields(t *testing.T) {
	tests := []struct {
		name string
		in   params
		want map[string]any
	}{
		{
			name: "omitted fields are dropped",
			in:   params{Name: param.Field[string]{Value: "a", Present: true}},
			want: map[string]any{"name": "a"},
		},
		{
			name: "explicit null and zero",
			in: params{
				Count: param.Field[int64]{Present: true},
				Note:  param.Field[string]{Null: true, Present: true},
			},
			want: map[string]any{"count": float64(0), "note": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("invalid JSON %s: %v", b, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %s, want %v", b, tt.want)
			}
			for k, v := range tt.want {
				if gv, ok := got[k]; !ok || gv != v {
					t.Fatalf("got %s, want %v", b, tt.want)
				}
			}
		})
	}
}
