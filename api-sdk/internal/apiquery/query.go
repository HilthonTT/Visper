package apiquery

import (
	"net/url"
	"reflect"
	"time"
)

type Queryer interface {
	URLQuery() url.Values
}

type QuerySettings struct {
	NestedFormat NestedQueryFormat
	ArrayFormat  ArrayQueryFormat
}

type NestedQueryFormat int

const (
	NestedQueryFormatBrackets NestedQueryFormat = iota
	NestedQueryFormatDots
)

type ArrayQueryFormat int

const (
	ArrayQueryFormatComma ArrayQueryFormat = iota
	ArrayQueryFormatRepeat
	ArrayQueryFormatIndices
	ArrayQueryFormatBrackets
)

func MarshalWithSettings(value any, settings QuerySettings) url.Values {
	e := encoder{
		dateFormat: time.RFC3339,
		root:       true,
		settings:   settings,
	}
	kv := url.Values{}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil
	}

	typ := val.Type()
	for _, pair := range e.typeEncoder(typ)("", val) {
		kv.Add(pair.key, pair.value)
	}
	return kv
}

func Marshal(value any) url.Values {
	return MarshalWithSettings(value, QuerySettings{})
}
