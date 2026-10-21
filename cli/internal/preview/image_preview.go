// Package preview renders images for display in the terminal.
package preview

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"strconv"
	"strings"

	"github.com/muesli/termenv"
	_ "golang.org/x/image/webp" // Register WebP decoder
)

// ImagePreviewer renders images as ANSI half-block art.
type ImagePreviewer struct{}

// NewImagePreviewer returns an ImagePreviewer.
func NewImagePreviewer() *ImagePreviewer {
	return &ImagePreviewer{}
}

type colorCache struct {
	rgbaToTermenv map[color.RGBA]termenv.RGBColor
}

func newColorCache() *colorCache {
	return &colorCache{
		rgbaToTermenv: make(map[color.RGBA]termenv.RGBColor),
	}
}

func (c *colorCache) getTermenvColor(col color.Color, fallbackColor string) termenv.RGBColor {
	rgba, ok := color.RGBAModel.Convert(col).(color.RGBA)
	if !ok || rgba.A == 0 {
		return termenv.RGBColor(fallbackColor)
	}

	if termenvColor, exists := c.rgbaToTermenv[rgba]; exists {
		return termenvColor
	}

	termenvColor := termenv.RGBColor(fmt.Sprintf("#%02x%02x%02x", rgba.R, rgba.G, rgba.B))
	c.rgbaToTermenv[rgba] = termenvColor
	return termenvColor
}

func ConvertImageToANSI(img image.Image, defaultBGColor color.Color) string {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	var output strings.Builder
	cache := newColorCache()
	defaultBGHex := colorToHex(defaultBGColor)
	bgColor := cache.getTermenvColor(defaultBGColor, "")

	for y := 0; y < height; y += 2 {
		for x := range width {
			upperColor := cache.getTermenvColor(img.At(x, y), defaultBGHex)

			// An odd final row has no lower pixel: fill it with the background.
			lowerColor := bgColor
			if y+1 < height {
				lowerColor = cache.getTermenvColor(img.At(x, y+1), defaultBGHex)
			}

			// Using the "▄" character which fills the lower half
			cell := termenv.String("▄").Foreground(lowerColor).Background(upperColor)
			output.WriteString(cell.String())
		}
		// Only add newline if this is not the last row
		if y+2 < height {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func ConvertImageToANSITransparent(img image.Image) string {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	var output strings.Builder
	cache := newColorCache()

	for y := 0; y < height; y += 2 {
		for x := range width {
			upperPixel := img.At(x, y)
			var lowerPixel color.Color

			if y+1 < height {
				lowerPixel = img.At(x, y+1)
			} else {
				lowerPixel = color.RGBA{R: 0, G: 0, B: 0, A: 0}
			}

			// Check transparency for both pixels (including near-black pixels)
			upperTransparent := isTransparentOrBlack(upperPixel)
			lowerTransparent := isTransparentOrBlack(lowerPixel)

			// Both transparent - output space (let terminal background show through)
			if upperTransparent && lowerTransparent {
				output.WriteByte(' ')
				continue
			}

			// Only upper transparent - show lower half block
			if upperTransparent {
				lowerColor := cache.getTermenvColor(lowerPixel, "")
				cell := termenv.String("▄").Foreground(lowerColor)
				output.WriteString(cell.String())
				continue
			}

			// Only lower transparent - show upper half block
			if lowerTransparent {
				upperColor := cache.getTermenvColor(upperPixel, "")
				cell := termenv.String("▀").Foreground(upperColor)
				output.WriteString(cell.String())
				continue
			}

			// Both opaque - standard rendering
			upperColor := cache.getTermenvColor(upperPixel, "")
			lowerColor := cache.getTermenvColor(lowerPixel, "")
			cell := termenv.String("▄").Foreground(lowerColor).Background(upperColor)
			output.WriteString(cell.String())
		}
		// Only add newline if this is not the last row
		if y+2 < height {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func (p *ImagePreviewer) ImagePreviewFromBytes(
	data []byte,
	maxWidth int,
	maxHeight int,
	defaultBGColor string,
) (string, error) {
	if maxWidth <= 0 || maxHeight <= 0 {
		return "", fmt.Errorf("dimensions must be positive (maxWidth=%d, maxHeight=%d)", maxWidth, maxHeight)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("image data is empty")
	}

	const maxFileSize = 100 * 1024 * 1024
	if len(data) >= maxFileSize {
		return "", fmt.Errorf("image data too large: %d bytes", len(data))
	}

	img, _, _, err := prepareImageForPreview(data)
	if err != nil {
		return "", err
	}

	if defaultBGColor == "" {
		return p.ANSIRendererTransparent(img, maxWidth, maxHeight)
	}

	return p.ANSIRenderer(img, defaultBGColor, maxWidth, maxHeight)
}

func (p *ImagePreviewer) ANSIRendererTransparent(img image.Image, maxWidth int, maxHeight int) (string, error) {
	fittedImg := resizeForANSI(img, maxWidth, maxHeight)
	return ConvertImageToANSITransparent(fittedImg), nil
}

// ImagePreviewWithRenderer generates an image preview using the specified renderer
func (p *ImagePreviewer) ANSIRenderer(
	img image.Image,
	defaultBGColor string,
	maxWidth,
	maxHeight int) (string, error) {
	bgColor, err := hexToColor(defaultBGColor)
	if err != nil {
		return "", fmt.Errorf("invalid background color: %w", err)
	}

	// For ANSI rendering, resize image appropriately
	fittedImg := resizeForANSI(img, maxWidth, maxHeight)
	return ConvertImageToANSI(fittedImg, bgColor), nil
}

func hexToColor(hex string) (color.RGBA, error) {
	if len(hex) != 7 || hex[0] != '#' {
		return color.RGBA{}, errors.New("invalid hex color format")
	}
	values, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: uint8(values >> 16), G: uint8((values >> 8) & 0xFF), B: uint8(values & 0xFF), A: 255}, nil
}

// colorToHex formats c as "#rrggbb", the form termenv.RGBColor understands.
func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
