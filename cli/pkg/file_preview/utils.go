package filepreview

import (
	"image/color"
	"log/slog"
	"runtime"
	"sync"
)

// Terminal cell to pixel conversion constants
// These approximate the pixel dimensions of terminal cells
const (
	DefaultPixelsPerColumn = 10 // approximate pixels per terminal column
	DefaultPixelsPerRow    = 20 // approximate pixels per terminal row
)

type TerminalCellSize struct {
	PixelsPerColumn int
	PixelsPerRow    int
}

type TerminalCapabilities struct {
	cellSize       TerminalCellSize
	cellSizeInit   sync.Once
	detectionMutex sync.Mutex
}

func NewTerminalCapabilities() *TerminalCapabilities {
	return &TerminalCapabilities{
		cellSize: TerminalCellSize{
			PixelsPerColumn: DefaultPixelsPerColumn,
			PixelsPerRow:    DefaultPixelsPerRow,
		},
	}
}

func (tc *TerminalCapabilities) InitTerminalCapabilities() {
	// Use a goroutine to avoid blocking the application startup
	go func() {
		// Initialize cell size detection
		tc.cellSizeInit.Do(func() {
			tc.cellSize = tc.detectTerminalCellSize()
			slog.Info("Terminal cell size detection",
				"pixels_per_column", tc.cellSize.PixelsPerColumn,
				"pixels_per_row", tc.cellSize.PixelsPerRow)
		})
	}()
}

func (tc *TerminalCapabilities) GetTerminalCellSize() TerminalCellSize {
	tc.cellSizeInit.Do(func() {
		tc.cellSize = tc.detectTerminalCellSize()
		slog.Info("Terminal cell size detection (lazy init)",
			"pixels_per_column", tc.cellSize.PixelsPerColumn,
			"pixels_per_row", tc.cellSize.PixelsPerRow)
	})

	return tc.cellSize
}

func (tc *TerminalCapabilities) detectTerminalCellSize() TerminalCellSize {
	tc.detectionMutex.Lock()
	defer tc.detectionMutex.Unlock()

	if runtime.GOOS == "windows" {
		if cellSize, ok := getTerminalCellSizeWindows(); ok {
			slog.Info("Successfully detected terminal cell size on Windows",
				"pixels_per_column", cellSize.PixelsPerColumn,
				"pixels_per_row", cellSize.PixelsPerRow)
			return cellSize
		}
	} else {
		// Unix-like systems (Linux, macOS, etc.)
		if cellSize, ok := getTerminalCellSizeViaIoctl(); ok {
			slog.Info("Successfully detected terminal cell size via ioctl",
				"pixels_per_column", cellSize.PixelsPerColumn,
				"pixels_per_row", cellSize.PixelsPerRow)
			return cellSize
		}
	}

	// Fallback to default values
	slog.Info("Using default terminal cell size", "os", runtime.GOOS)
	return getDefaultCellSize()
}

// Windows-specific terminal detection functions
// getTerminalCellSizeWindows uses Windows Console API to detect terminal cell size
func getTerminalCellSizeWindows() (TerminalCellSize, bool) {
	if runtime.GOOS != "windows" {
		return TerminalCellSize{}, false
	}

	// For Windows, just return reasonable defaults
	// Windows terminal detection is complex and varies greatly between
	// different terminal emulators (Windows Terminal, ConEmu, etc.)
	slog.Info("Using Windows default terminal cell size")
	// TODO: Implement actual Windows Console API calls when running on Windows
	return getWindowsDefaultCellSize(), true
}

// getWindowsDefaultCellSize returns reasonable defaults for Windows
func getWindowsDefaultCellSize() TerminalCellSize {
	return TerminalCellSize{
		PixelsPerColumn: 8,  // Windows Terminal/CMD typical width
		PixelsPerRow:    16, // Windows Terminal/CMD typical height
	}
}

func getDefaultCellSize() TerminalCellSize {
	return TerminalCellSize{
		PixelsPerColumn: DefaultPixelsPerColumn,
		PixelsPerRow:    DefaultPixelsPerRow,
	}
}

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
