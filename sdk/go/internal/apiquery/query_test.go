package apiquery

import (
	"testing"

	"github.com/hilthontt/visper/sdk/go/internal/param"
)

type listParams struct {
	Limit param.Field[int64]    `query:"limit"`
	Query param.Field[string]   `query:"q"`
	Tags  param.Field[[]string] `query:"tags"`
	Inner param.Field[inner]    `query:"inner"`
}

type inner struct {
	A param.Field[string] `query:"a"`
}

func TestMarshalWithSettings(t *testing.T) {
	base := listParams{
		Limit: param.Field[int64]{Value: 10, Present: true},
		Query: param.Field[string]{Value: "a b&c", Present: true},
		Tags:  param.Field[[]string]{Value: []string{"x", "y"}, Present: true},
		Inner: param.Field[inner]{Value: inner{A: param.Field[string]{Value: "z", Present: true}}, Present: true},
	}

	tests := []struct {
		name     string
		settings QuerySettings
		want     string
	}{
		{
			name:     "comma arrays, bracket nesting",
			settings: QuerySettings{ArrayFormat: ArrayQueryFormatComma, NestedFormat: NestedQueryFormatBrackets},
			want:     "inner%5Ba%5D=z&limit=10&q=a+b%26c&tags=x%2Cy",
		},
		{
			name:     "repeated arrays, dot nesting",
			settings: QuerySettings{ArrayFormat: ArrayQueryFormatRepeat, NestedFormat: NestedQueryFormatDots},
			want:     "inner.a=z&limit=10&q=a+b%26c&tags=x&tags=y",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MarshalWithSettings(base, tt.settings).Encode(); got != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestMarshalOmitsUnsetFields(t *testing.T) {
	if got := Marshal(listParams{}).Encode(); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestMarshalSmallNumericKinds(t *testing.T) {
	type params struct {
		F float32 `query:"f"`
		I int8    `query:"i"`
	}
	got := Marshal(params{F: 0.1, I: -3}).Encode()
	if want := "f=0.1&i=-3"; got != want {
		t.Fatalf("Marshal = %q, want %q", got, want)
	}
}
