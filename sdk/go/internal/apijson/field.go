package apijson

import "reflect"

type status uint8

const (
	missing status = iota
	null
	invalid
	valid
)

type Field struct {
	raw    string
	status status
}

// Returns true if the field is explicitly `null` _or_ if it is not present at all (ie, missing).
// To check if the field's key is present in the JSON with an explicit null value,
// you must check `f.IsNull() && !f.IsMissing()`.
func (f Field) IsNull() bool {
	return f.status <= null
}

func (f Field) IsMissing() bool {
	return f.status == missing
}

func (f Field) IsInvalid() bool {
	return f.status == invalid
}

func (f Field) Raw() string {
	return f.raw
}

func getSubField(root reflect.Value, index []int, name string) reflect.Value {
	strct := root.FieldByIndex(index[:len(index)-1])
	if !strct.IsValid() {
		panic("couldn't find encapsulating struct for field " + name)
	}

	meta := strct.FieldByName("JSON")
	if !meta.IsValid() {
		return reflect.Value{}
	}

	field := meta.FieldByName("JSON")
	if !meta.IsValid() {
		return reflect.Value{}
	}

	return field
}
