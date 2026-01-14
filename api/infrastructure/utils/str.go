package utils

import (
	"encoding/base64"
	"strings"
)

var filenameCharMapping = map[string]string{
	"/":  "_",
	"\\": "_",
	":":  "-",
	"*":  "_",
	"?":  "_",
	"\"": "_",
	"<":  "_",
	">":  "_",
	"|":  "_",
	" ":  "_",
}

var DEC = map[string]string{
	"-": "+",
	"_": "/",
	".": "=",
}

func MappingName(name string) string {
	for k, v := range filenameCharMapping {
		name = strings.ReplaceAll(name, k, v)
	}
	return name
}

func SafeAtob(data string) (string, error) {
	for k, v := range DEC {
		data = strings.ReplaceAll(data, k, v)
	}
	bytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return string(bytes), err
}

// GetNoneEmpty returns the first non-empty string, return empty if all empty
func GetNoneEmpty(strArr ...string) string {
	for _, s := range strArr {
		if len(s) > 0 {
			return s
		}
	}
	return ""
}
