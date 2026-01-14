package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

var (
	// MD5 indicates MD5 support
	MD5 = RegisterHash("md5", "MD5", 32, md5.New)

	// SHA1 indicates SHA-1 support
	SHA1 = RegisterHash("sha1", "SHA-1", 40, sha1.New)

	// SHA256 indicates SHA-256 support
	SHA256 = RegisterHash("sha256", "SHA-256", 64, sha256.New)
)

var (
	name2hash  = map[string]*HashType{}
	alias2hash = map[string]*HashType{}
	Supported  []*HashType
)

type HashType struct {
	Width   int
	Name    string
	Alias   string
	NewFunc func(...any) hash.Hash
}

func (ht *HashType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ht.Name + `"`), nil
}

func (ht *HashType) MarshalText() (text []byte, err error) {
	return []byte(ht.Name), nil
}

type HashInfo struct {
	H map[*HashType]string `json:"hashInfo"`
}

func NewHashInfoByMap(h map[*HashType]string) HashInfo {
	return HashInfo{h}
}

func NewHashInfo(ht *HashType, str string) HashInfo {
	m := make(map[*HashType]string)
	if ht != nil {
		m[ht] = str
	}
	return HashInfo{H: m}
}

func GetMD5EncodeStr(data string) string {
	return HashData(MD5, []byte(data))
}

func HashData(hashType *HashType, data []byte, params ...any) string {
	h := hashType.NewFunc(params...)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// RegisterHash adds a new Hash to the list and returns its Type
func RegisterHash(name, alias string, width int, newFunc func() hash.Hash) *HashType {
	return RegisterHashWithParam(name, alias, width, func(a ...any) hash.Hash {
		return newFunc()
	})
}

func RegisterHashWithParam(name, alias string, width int, newFunc func(...any) hash.Hash) *HashType {
	newType := &HashType{
		Name:    name,
		Alias:   alias,
		Width:   width,
		NewFunc: newFunc,
	}

	name2hash[name] = newType
	alias2hash[alias] = newType
	Supported = append(Supported, newType)
	return newType
}

// HashReader get hash of one hashType from a reader
func HashReader(hashType *HashType, reader io.Reader, params ...any) (string, error) {
	h := hashType.NewFunc(params...)
	_, err := CopyWithBuffer(h, reader)
	if err != nil {
		return "", fmt.Errorf("HashReader error: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile get hash of one hashType from a model.File
func HashFile(hashType *HashType, file io.ReadSeeker, params ...any) (string, error) {
	str, err := HashReader(hashType, file, params...)
	if err != nil {
		return "", err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return str, err
	}
	return str, nil
}
