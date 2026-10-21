package preview

import "image/color"

func isTransparentOrBlack(c color.Color) bool {
	r, g, b, a := c.RGBA()

	// Check if transparent
	if a == 0 {
		return true
	}

	// Check if near-black (adjust threshold as needed)
	// Values are in range 0-65535, so we convert to 0-255 range
	const threshold = 20 // Adjust this value (0-255) to be more/less aggressive

	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)

	// Consider it "black" if all RGB components are below threshold
	return r8 < threshold && g8 < threshold && b8 < threshold
}
