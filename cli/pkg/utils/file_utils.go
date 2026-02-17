package utils

import (
	"io/fs"
	"log/slog"
	"path/filepath"
)

func DirSize(path string) int64 {
	var size int64

	walkErr := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Error("Dir size function error", "error", err)
		}

		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr == nil {
				size += info.Size()
			}
		}
		return err
	})
	if walkErr != nil {
		slog.Error("errors during WalkDir", "error", walkErr)
	}
	return size
}
