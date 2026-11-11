package apijson

import (
	"math"
	"reflect"
	"testing"

	"github.com/tidwall/gjson"
)

func TestMarshalRejectsNonFiniteFloats(t *testing.T) {
	type body struct {
		X float64 `json:"x"`
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if out, err := Marshal(body{X: f}); err == nil {
			t.Errorf("Marshal(%v) = %s, want error (not valid JSON)", f, out)
		}
	}
	type small struct {
		F float32 `json:"f"`
		I int8    `json:"i"`
	}
	out, err := Marshal(small{F: 0.1, I: -3})
	if err != nil || string(out) != `{"f":0.1,"i":-3}` {
		t.Fatalf("Marshal = %s, %v", out, err)
	}
}

type unionAB interface{ isAB() }

type enumA string

func (enumA) isAB()           {}
func (e enumA) IsKnown() bool { return e == "a" }

type enumB string

func (enumB) isAB()           {}
func (e enumB) IsKnown() bool { return e == "b" }

type withUnion struct {
	V unionAB `json:"v"`
}

func TestUnionPrefersExactVariant(t *testing.T) {
	RegisterUnion(reflect.TypeOf((*unionAB)(nil)).Elem(), "",
		UnionVariant{TypeFilter: gjson.String, Type: reflect.TypeOf(enumA(""))},
		UnionVariant{TypeFilter: gjson.String, Type: reflect.TypeOf(enumB(""))},
	)
	var got withUnion
	if err := Unmarshal([]byte(`{"v":"b"}`), &got); err != nil {
		t.Fatal(err)
	}
	// enumA accepts "b" only loosely (unknown enum value); enumB is exact.
	if _, ok := got.V.(enumB); !ok {
		t.Fatalf("decoded %T(%v), want enumB", got.V, got.V)
	}
}

func TestUnmarshalFixedSizeArray(t *testing.T) {
	var got struct {
		A [3]int `json:"a"`
	}
	got.A = [3]int{9, 9, 9}
	if err := Unmarshal([]byte(`{"a":[1,2]}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.A != [3]int{1, 2, 0} {
		t.Fatalf("A = %v, want [1 2 0]", got.A)
	}
}

type withMeta struct {
	ID   string       `json:"id"`
	Name string       `json:"name"`
	JSON withMetaJSON `json:"-"`
}

type withMetaJSON struct {
	ID          Field
	Name        Field
	raw         string
	ExtraFields map[string]Field
}

func TestUnmarshalPopulatesFieldMetadata(t *testing.T) {
	var got withMeta
	if err := Unmarshal([]byte(`{"id":"x","name":null,"other":1}`), &got); err != nil {
		t.Fatal(err)
	}
	m := got.JSON
	if m.ID.IsMissing() || m.ID.IsNull() || m.ID.Raw() != `"x"` {
		t.Errorf("ID metadata = %+v, want valid", m.ID)
	}
	if !m.Name.IsNull() || m.Name.IsMissing() {
		t.Errorf("Name metadata = %+v, want explicit null", m.Name)
	}
	if _, ok := m.ExtraFields["other"]; !ok {
		t.Errorf("ExtraFields = %v, want key other", m.ExtraFields)
	}
	if m.raw == "" {
		t.Error("raw JSON not recorded")
	}
}
