package apiquery

import (
	"reflect"
	"strings"
)

const queryStructTag = "query"
const formatStructTag = "format"

type parsedStructTag struct {
	name      string
	omitempty bool
	inline    bool
}

func parseQueryStructTag(field reflect.StructField) (parsedStructTag, bool) {
	tag := parsedStructTag{}

	raw, ok := field.Tag.Lookup(queryStructTag)
	if !ok {
		return tag, false
	}

	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return tag, false
	}

	tag.name = parts[0]
	for _, part := range parts {
		switch part {
		case "omitempty":
			tag.omitempty = true
		case "inline":
			tag.inline = true
		}
	}

	return tag, true
}

func parseFormatStructTag(field reflect.StructField) (string, bool) {
	format, ok := field.Tag.Lookup(formatStructTag)
	return format, ok
}
