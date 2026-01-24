package qrfefe

import (
	sgr "github.com/foize/go.sgr"
	"rsc.io/qr"
)

var tops_bottoms = []rune{' ', '▀', '▄', '█'}

// Generate a text string to a QR code, which you can write to a terminal or file.
// Generate is a shorthand for DefaultConfig.Generate(text)
func Generate(size int, text string) (string, int, error) {
	return generate(qr.L, text)
}

func generate(level qr.Level, text string) (string, int, error) {
	code, err := qr.Encode(text, qr.Level(level))
	if err != nil {
		return "", 0, err
	}

	qrRunes := make([]rune, 0)

	for y := 0; y < code.Size-1; y += 2 {
		qrRunes = append(qrRunes, []rune(sgr.FgWhite+sgr.BgBlack)...)

		for x := 0; x < code.Size; x += 1 {
			var num int8
			if code.Black(x, y) { // top pixel black
				num += 1
			}

			if code.Black(x, y+1) { // bottom pixel black
				num += 2
			}
			qrRunes = append(qrRunes, tops_bottoms[num])
		}

		qrRunes = append(qrRunes, []rune(sgr.Reset)...)
		qrRunes = append(qrRunes, '\n')
	}

	addWhiteRow(&qrRunes, code.Size+4)

	return string(qrRunes), code.Size, nil
}

func addWhiteRow(qrRunes *[]rune, width int) {
	if qrRunes == nil {
		return
	}

	*qrRunes = append(*qrRunes, []rune(sgr.FgWhite+sgr.BgBlack)...)
	for i := 1; i < width-3; i++ {
		*qrRunes = append(*qrRunes, '▀')
	}
	*qrRunes = append(*qrRunes, []rune(sgr.Reset)...)
	*qrRunes = append(*qrRunes, '\n')
}
