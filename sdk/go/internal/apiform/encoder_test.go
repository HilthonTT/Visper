package apiform

import (
	"bytes"
	"io"
	"mime/multipart"
	"strings"
	"testing"
)

func TestMarshalOmitsNilReader(t *testing.T) {
	type upload struct {
		Name string    `form:"name"`
		File io.Reader `form:"file"`
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := Marshal(upload{Name: "x"}, w); err != nil { // panicked on the nil reader
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `name="file"`) {
		t.Fatalf("nil file was encoded:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `name="name"`) {
		t.Fatalf("name field missing:\n%s", buf.String())
	}
}
