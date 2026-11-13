package preview

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestHexToColor(t *testing.T) {
	tests := []struct {
		in      string
		want    color.RGBA
		wantErr bool
	}{
		{"#000000", color.RGBA{A: 255}, false},
		{"#ff8000", color.RGBA{R: 255, G: 128, A: 255}, false},
		{"ff8000", color.RGBA{}, true},
		{"#ff80", color.RGBA{}, true},
		{"#gg0000", color.RGBA{}, true},
	}
	for _, tt := range tests {
		got, err := hexToColor(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("hexToColor(%q) = %v, %v; want %v, err %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestColorToHexRoundTrip(t *testing.T) {
	for _, hex := range []string{"#000000", "#ffffff", "#123abc"} {
		c, err := hexToColor(hex)
		if err != nil {
			t.Fatal(err)
		}
		if got := colorToHex(c); got != hex {
			t.Errorf("colorToHex(hexToColor(%q)) = %q", hex, got)
		}
	}
}

func TestConvertImageToANSIRows(t *testing.T) {
	tests := []struct {
		w, h      int
		wantLines int
	}{
		{4, 1, 1},
		{4, 2, 1},
		{4, 3, 2},
		{3, 6, 3},
	}
	for _, tt := range tests {
		img := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))
		out := ConvertImageToANSI(img, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		if got := len(strings.Split(out, "\n")); got != tt.wantLines {
			t.Errorf("%dx%d: got %d lines, want %d", tt.w, tt.h, got, tt.wantLines)
		}
		if got := strings.Count(out, "▄"); got != tt.w*tt.wantLines {
			t.Errorf("%dx%d: got %d cells, want %d", tt.w, tt.h, got, tt.w*tt.wantLines)
		}
	}
}

func TestAdjustOrientationDimensions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	tests := []struct {
		orientation int
		w, h        int
	}{
		{1, 4, 2}, {2, 4, 2}, {3, 4, 2}, {4, 4, 2},
		{5, 2, 4}, {6, 2, 4}, {7, 2, 4}, {8, 2, 4},
		{0, 4, 2}, {9, 4, 2},
	}
	for _, tt := range tests {
		b := adjustOrientation(img, tt.orientation).Bounds()
		if b.Dx() != tt.w || b.Dy() != tt.h {
			t.Errorf("orientation %d: got %dx%d, want %dx%d", tt.orientation, b.Dx(), b.Dy(), tt.w, tt.h)
		}
	}
}

func TestPrepareImageRejectsHugeDimensions(t *testing.T) {
	// A tiny PNG header declaring 30000x30000 pixels: rejected before decoding.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	binary.BigEndian.PutUint32(data[16:], 30000) // IHDR width
	binary.BigEndian.PutUint32(data[20:], 30000) // IHDR height
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("test image header is invalid: %v", err)
	}
	if _, _, _, err := prepareImageForPreview(data); err == nil {
		t.Fatal("expected an error for a 900-megapixel image")
	}
}
