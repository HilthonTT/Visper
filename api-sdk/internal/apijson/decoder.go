package apijson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

type exactness int8

const (
	// Some values had to fudged a bit, for example by converting a string to an
	// int, or an enum with extra values.
	loose exactness = iota
	// There are some extra arguments, but other wise it matches the union.
	extras
	// Exactly right.
	exact
)

var decoders sync.Map

// decoderBuilder contains the 'compile-time' state of the decoder.
type decoderBuilder struct {
	// Whether or not this is the first element and called by [UnmarshalRoot], see
	// the documentation there to see why this is necessary.
	root bool

	// The dateFormat (a format string for [time.Format]) which is chosen by the
	// last struct tag that was seen.
	dateFormat string
}

type decoderState struct {
	strict    bool
	exactness exactness
}

type decoderFunc func(node gjson.Result, value reflect.Value, state *decoderState) error

type decoderField struct {
	tag    parsedStructTag
	fn     decoderFunc
	idx    []int
	goname string
}

type decoderEntry struct {
	reflect.Type
	dateFormat string
	root       bool
}

func (d *decoderBuilder) unmarshal(raw []byte, to any) error {
	value := reflect.ValueOf(to).Elem()
	result := gjson.ParseBytes(raw)
	if !value.IsValid() {
		return fmt.Errorf("apijson: cannot marshal into invalid value")
	}
	return d.typeDecoder(value.Type())(result, value, &decoderState{strict: false, exactness: exact})
}

func Unmarshal(raw []byte, to any) error {
	d := &decoderBuilder{dateFormat: time.RFC3339}
	return d.unmarshal(raw, to)
}

// UnmarshalRoot is like Unmarshal, but doesn't try to call MarshalJSON on the
// root element. Useful if a struct's UnmarshalJSON is overrode to use the
// behavior of this encoder versus the standard library.
func UnmarshalRoot(raw []byte, to any) error {
	d := &decoderBuilder{dateFormat: time.RFC3339, root: true}
	return d.unmarshal(raw, to)
}

func (d *decoderBuilder) typeDecoder(t reflect.Type) decoderFunc {
	entry := decoderEntry{
		Type:       t,
		dateFormat: d.dateFormat,
		root:       d.root,
	}

	if fi, ok := decoders.Load(entry); ok {
		return fi.(decoderFunc)
	}

	// To deal with recursive types, populate the map with an
	// indirect func before we build it. This type waits on the
	// real func (f) to be ready and then calls it. This indirect
	// func is only used for recursive types.
	var (
		wg sync.WaitGroup
		f  decoderFunc
	)

	wg.Add(1)
	fi, loaded := decoders.LoadOrStore(entry, decoderFunc(func(node gjson.Result, v reflect.Value, state *decoderState) error {
		wg.Wait()
		return f(node, v, state)
	}))
	if loaded {
		return fi.(decoderFunc)
	}

	// Compute the real decoder and replace the indirect func with it.
	f = d.newTypeDecoder(t)
	wg.Done()
	decoders.Store(entry, f)
	return f
}

func indirectUnmarshalerDecoder(n gjson.Result, v reflect.Value, state *decoderState) error {
	return v.Addr().Interface().(json.Unmarshaler).UnmarshalJSON([]byte(n.Raw))
}

func unmarshalerDecoder(n gjson.Result, v reflect.Value, state *decoderState) error {
	if v.Kind() == reflect.Pointer && v.CanSet() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return v.Interface().(json.Unmarshaler).UnmarshalJSON([]byte(n.Raw))
}

func (d *decoderBuilder) newTypeDecoder(t reflect.Type) decoderFunc {
	if t.ConvertibleTo(reflect.TypeOf(time.Time{})) {
		return d.newTimeTypeDecoder(t)
	}
	if !d.root && t.Implements(reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()) {
		return unmarshalerDecoder
	}
	if !d.root && reflect.PointerTo(t).Implements(reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()) {
		if _, ok := unionVariants[t]; !ok {
			return indirectUnmarshalerDecoder
		}
	}
	d.root = false

	if _, ok := unionRegistry[t]; ok {
		return d.newUnionDecoder(t)
	}

	switch t.Kind() {
	case reflect.Pointer:
		inner := t.Elem()
		innerDecoder := d.typeDecoder(inner)

		return func(n gjson.Result, v reflect.Value, state *decoderState) error {
			if !v.IsValid() {
				return fmt.Errorf("apijson: unexpected invalid reflection value %+#v", v)
			}

			newValue := reflect.New(inner).Elem()
			err := innerDecoder(n, newValue, state)
			if err != nil {
				return err
			}

			v.Set(newValue.Addr())
			return nil
		}
	case reflect.Struct:
		return d.newStructTypeDecoder(t)
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		return d.newArrayTypeDecoder(t)
	case reflect.Map:
		return d.newMapDecoder(t)
	case reflect.Interface:
		return func(node gjson.Result, value reflect.Value, state *decoderState) error {
			if !value.IsValid() {
				return fmt.Errorf("apijson: unexpected invalid value %+#v", value)
			}
			if node.Value() != nil && value.CanSet() {
				value.Set(reflect.ValueOf(node.Value()))
			}
			return nil
		}
	default:
		return d.newPrimitiveTypeDecoder(t)
	}
}
