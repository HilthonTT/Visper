//go:build !unix

package tui

import (
	"os"
)

func getOwnerAndGroup(_ os.FileInfo) (string, string) {
	return "", ""
}
