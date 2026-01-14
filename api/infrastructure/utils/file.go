package utils

import (
	"os"
	"strings"

	"github.com/hilthontt/visper/api/infrastructure/config"
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
