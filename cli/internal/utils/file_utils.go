package utils

import (
	"io/fs"
	"log/slog"
	"path/filepath"
)

// DirSize returns the total size in bytes of all regular files below path.
// Unreadable entries are logged and skipped.
func DirSize(path string) int64 {
	var size int64

	walkErr := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// d may be nil here (e.g. the root could not be stat'ed); skip the
			// entry but keep walking the rest of the tree.
			slog.Error("Dir size function error", "path", path, "error", err)
			return nil
		}

		if !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				size += info.Size()
			}
		}
		return nil
	})
	if walkErr != nil {
		slog.Error("errors during WalkDir", "error", walkErr)
	}
	return size
}
