package filepreview

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/muesli/termenv"
	_ "golang.org/x/image/webp" // Register WebP decoder
)

type ImageRenderer int

const (
	RendererANSI ImageRenderer = iota
	RendererKitty
)

type CachedPreview struct {
	Preview    string
	Timestamp  time.Time
	Renderer   ImageRenderer
	Dimensions string // "width,height,bgColor,sideAreaWidth"
}

type ImagePreviewCache struct {
	cache      map[string]*CachedPreview
	mutex      sync.RWMutex
	maxEntries int
	expiration time.Duration
}

type ImagePreviewer struct {
	cache       *ImagePreviewCache
	terminalCap *TerminalCapabilities
}

func NewImagePreviewer() *ImagePreviewer {
	return NewImagePreviewerWithConfig(100, 5*time.Minute)
}

func NewImagePreviewerWithConfig(maxEntries int, expiration time.Duration) *ImagePreviewer {
	previewer := &ImagePreviewer{
		cache:       NewImagePreviewCache(maxEntries, expiration),
		terminalCap: NewTerminalCapabilities(),
	}

	// Initialize terminal capabilities
	previewer.terminalCap.InitTerminalCapabilities()

	return previewer
}

func NewImagePreviewCache(maxEntries int, expiration time.Duration) *ImagePreviewCache {
	cache := &ImagePreviewCache{
		cache:      make(map[string]*CachedPreview),
		maxEntries: maxEntries,
		expiration: expiration,
	}

	// Start a cleanup goroutine
	go cache.periodicCleanup()

	return cache
}

func (c *ImagePreviewCache) periodicCleanup() {
	ticker := time.NewTicker(c.expiration / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpired()
	}
}

func (c *ImagePreviewCache) cleanupExpired() {
	now := time.Now()
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for key, entry := range c.cache {
		if now.Sub(entry.Timestamp) > c.expiration {
			delete(c.cache, key)
		}
	}
}

func (c *ImagePreviewCache) Get(path, dimensions string, renderer ImageRenderer) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	cacheKey := path + ":" + dimensions

	if entry, exists := c.cache[cacheKey]; exists {
		if entry.Renderer == renderer && time.Since(entry.Timestamp) < c.expiration {
			return entry.Preview, true
		}
	}

	return "", false
}

func (c *ImagePreviewCache) Set(path, dimensions, preview string, renderer ImageRenderer) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if we need to evict entries
	if len(c.cache) >= c.maxEntries {
		c.evictOldest()
	}

	cacheKey := path + ":" + dimensions
	c.cache[cacheKey] = &CachedPreview{
		Preview:    preview,
		Timestamp:  time.Now(),
		Renderer:   renderer,
		Dimensions: dimensions,
	}
}

func (c *ImagePreviewCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
		}
	}

	// Remove the oldest entry
	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
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
	output := ""
	cache := newColorCache()
	defaultBGHex := colorToHex(defaultBGColor)

	lowerColor := cache.getTermenvColor(defaultBGColor, "")

	for y := 0; y < height; y += 2 {
		for x := range width {
			upperColor := cache.getTermenvColor(img.At(x, y), defaultBGHex)

			if y+1 < height {
				lowerColor = cache.getTermenvColor(img.At(x, y+1), defaultBGHex)
			}

			// Using the "▄" character which fills the lower half
			cell := termenv.String("▄").Foreground(lowerColor).Background(upperColor)
			output += cell.String()
		}
		// Only add newline if this is not the last row
		if y+2 < height {
			output += "\n"
		}
	}
	return output
}

func ConvertImageToANSITransparent(img image.Image) string {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	output := ""
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
				output += " "
				continue
			}

			// Only upper transparent - show lower half block
			if upperTransparent {
				lowerColor := cache.getTermenvColor(lowerPixel, "")
				cell := termenv.String("▄").Foreground(lowerColor)
				output += cell.String()
				continue
			}

			// Only lower transparent - show upper half block
			if lowerTransparent {
				upperColor := cache.getTermenvColor(upperPixel, "")
				cell := termenv.String("▀").Foreground(upperColor)
				output += cell.String()
				continue
			}

			// Both opaque - standard rendering
			upperColor := cache.getTermenvColor(upperPixel, "")
			lowerColor := cache.getTermenvColor(lowerPixel, "")
			cell := termenv.String("▄").Foreground(lowerColor).Background(upperColor)
			output += cell.String()
		}
		// Only add newline if this is not the last row
		if y+2 < height {
			output += "\n"
		}
	}
	return output
}

func (p *ImagePreviewer) ImagePreview(
	path string,
	maxWidth int,
	maxHeight int,
	defaultBGColor string,
	sideAreaWidth int) (string, error) {
	// Validate dimensions
	if maxWidth <= 0 || maxHeight <= 0 {
		return "", fmt.Errorf("dimensions must be positive (maxWidth=%d, maxHeight=%d)", maxWidth, maxHeight)
	}

	// Create dimensions string for cache key
	dimensions := fmt.Sprintf("%d,%d,%s,%d", maxWidth, maxHeight, defaultBGColor, sideAreaWidth)

	// Try Kitty first as it's more modern
	if p.IsKittyCapable() {
		// Check cache for Kitty renderer
		if preview, found := p.cache.Get(path, dimensions, RendererKitty); found {
			return preview, nil
		}

		preview, err := p.ImagePreviewWithRenderer(
			path,
			maxWidth,
			maxHeight,
			defaultBGColor,
			RendererKitty,
			sideAreaWidth,
		)
		if err == nil {
			// Cache the successful result
			p.cache.Set(path, dimensions, preview, RendererKitty)
			return preview, nil
		}

		// Fall through to ANSI if Kitty fails
		slog.Error("Kitty renderer failed, falling back to ANSI", "error", err)
	}

	// Check cache for ANSI renderer
	if preview, found := p.cache.Get(path, dimensions, RendererANSI); found {
		return preview, nil
	}

	// Fall back to ANSI
	preview, err := p.ImagePreviewWithRenderer(path, maxWidth, maxHeight, defaultBGColor, RendererANSI, sideAreaWidth)
	if err == nil {
		// Cache the successful result
		p.cache.Set(path, dimensions, preview, RendererANSI)
	}
	return preview, err
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
func (p *ImagePreviewer) ImagePreviewWithRenderer(
	path string,
	maxWidth int,
	maxHeight int,
	defaultBGColor string,
	renderer ImageRenderer,
	sideAreaWidth int) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	const maxFileSize = 100 * 1024 * 1024 // 100MB limit
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("image file too large: %d bytes", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Use the new image preparation pipeline
	img, originalWidth, originalHeight, err := prepareImageForPreview(data)
	if err != nil {
		return "", err
	}

	switch renderer {
	case RendererKitty:
		result, err := p.renderWithKittyUsingTermCap(img, path, originalWidth,
			originalHeight, maxWidth, maxHeight, sideAreaWidth)
		if err != nil {
			// If kitty fails, fall back to ANSI renderer
			slog.Error("Kitty renderer failed, falling back to ANSI", "error", err)
			return p.ANSIRenderer(img, defaultBGColor, maxWidth, maxHeight)
		}
		return result, nil

	case RendererANSI:
		return p.ANSIRenderer(img, defaultBGColor, maxWidth, maxHeight)
	default:
		return "", fmt.Errorf("invalid renderer : %v", renderer)
	}
}

// Convert image to ansi
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

func colorToHex(color color.Color) string {
	r, g, b, a := color.RGBA()
	return fmt.Sprintf("#%02x%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}
