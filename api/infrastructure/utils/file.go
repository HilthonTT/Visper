package utils

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"strings"

	"github.com/hilthontt/visper/api/infrastructure/config"
)

const (
	TempDir = "temp"
)

const (
	KB = 1 << (10 * (iota + 1))
	MB
	GB
	TB
)

func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func GetFileType(filename string) int {
	ext := strings.ToLower(Ext(filename))
	if SliceContains(config.SlicesMap[config.AudioTypes], ext) {
		return config.AUDIO
	}
	if SliceContains(config.SlicesMap[config.VideoTypes], ext) {
		return config.VIDEO
	}
	if SliceContains(config.SlicesMap[config.ImageTypes], ext) {
		return config.IMAGE
	}
	if SliceContains(config.SlicesMap[config.TextTypes], ext) {
		return config.TEXT
	}
	return config.UNKNOWN
}

func CreateTempFile(r io.Reader, size int64) (*os.File, error) {
	if f, ok := r.(*os.File); ok {
		return f, nil
	}

	f, err := os.CreateTemp(TempDir, "file-*")
	if err != nil {
		return nil, err
	}
	readBytes, err := CopyWithBuffer(f, r)
	if err != nil {
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("CreateTempFile failed: %v", err)
	}
	if size > 0 && readBytes != size {
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("CreateTempFile failed, incoming stream actual size= %d, expect = %d: %v", readBytes, size, err)
	}
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("CreateTempFile failed, can't seek to 0: %v", err)
	}
	return f, nil
}

var extraMimeTypes = map[string]string{
	".apk": "application/vnd.android.package-archive",
}

func GetMimeType(name string) string {
	ext := path.Ext(name)
	if m, ok := extraMimeTypes[ext]; ok {
		return m
	}
	m := mime.TypeByExtension(ext)
	if m != "" {
		return m
	}
	return "application/octet-stream"
}
