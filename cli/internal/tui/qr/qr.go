// Package qr renders QR codes as text using Unicode half blocks.
package qr

import (
	"strings"

	sgr "github.com/foize/go.sgr"
	rscqr "rsc.io/qr"
)

// halfBlocks is indexed by (top black) | (bottom black)<<1.
var halfBlocks = []rune{' ', '▀', '▄', '█'}

// Generate encodes text as a QR code and renders it for a terminal, two
// modules per character row. It returns the rendering and the QR code's size
// in modules.
func Generate(text string) (string, int, error) {
	return generate(rscqr.L, text)
}

func generate(level rscqr.Level, text string) (string, int, error) {
	code, err := rscqr.Encode(text, level)
	if err != nil {
		return "", 0, err
	}

	var b strings.Builder

	// QR codes have an odd number of rows; the last pass pairs the final row
	// with an out-of-range (white) one, which Black reports as false.
	for y := 0; y < code.Size; y += 2 {
		b.WriteString(sgr.FgWhite + sgr.BgBlack)

		for x := 0; x < code.Size; x++ {
			var num int
			if code.Black(x, y) { // top pixel black
				num += 1
			}
			if code.Black(x, y+1) { // bottom pixel black
				num += 2
			}
			b.WriteRune(halfBlocks[num])
		}

		b.WriteString(sgr.Reset)
		b.WriteByte('\n')
	}

	addWhiteRow(&b, code.Size+4)

	return b.String(), code.Size, nil
}

// addWhiteRow appends a full-width row of upper half blocks.
func addWhiteRow(b *strings.Builder, width int) {
	b.WriteString(sgr.FgWhite + sgr.BgBlack)
	for i := 1; i < width-3; i++ {
		b.WriteRune('▀')
	}
	b.WriteString(sgr.Reset)
	b.WriteByte('\n')
}
