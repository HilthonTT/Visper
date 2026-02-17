//go:build windows

package tui

import (
	"os"
)

func getOwnerAndGroup(_ os.FileInfo) (string, string) {
	return "", ""
}
