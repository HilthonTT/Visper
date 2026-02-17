package tui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/barasher/go-exiftool"
	stringfunction "github.com/hilthontt/visper/cli/pkg/string_function"
	"github.com/hilthontt/visper/cli/pkg/utils"
)

const (
	fileStatErrorMsg  = "Cannot load file stats"
	linkFileBrokenMsg = "Link file is broken!"
	etFetchErrorMsg   = "Errors while fetching metadata via exiftool"

	keyName         = "Name"
	keySize         = "Size"
	keyDataModified = "Date Modified"
	keyDataAccessed = "Date Accessed"
	keyPermissions  = "Permissions"
	keyOwner        = "Owner"
	keyGroup        = "Group"
	keyPath         = "Path"
)

var sortPriority = map[string]int{ //nolint: gochecknoglobals // This is effectively const.
	keyName:         0,
	keySize:         1,
	keyDataModified: 2,
	keyDataAccessed: 3,
	keyPermissions:  4,
	keyOwner:        5,
	keyGroup:        6,
	keyPath:         7,
}

type Metadata struct {
	// Stores key value pairs
	data     [][2]string
	infoMsg  string
	filepath string
}

func NewMetadata(data [][2]string, filepath string, infoMsg string) Metadata {
	return Metadata{
		data:     data,
		filepath: filepath,
		infoMsg:  infoMsg,
	}
}

func (m Metadata) GetPath() string {
	return m.filepath
}

func (m Metadata) GetData() [][2]string {
	return m.data
}

func (m Metadata) GetValue(key string) (string, error) {
	for _, pair := range m.data {
		if pair[0] == key {
			return pair[1], nil
		}
	}

	return "", fmt.Errorf("key %s not found", key)
}

func GetMetadata(filepath string, metadatafocused bool, et *exiftool.Exiftool) Metadata {
	meta := getMetaDataUnsorted(filepath, metadatafocused, et)
	sortMetadata(meta.data)
	return meta
}

func getSymLinkMetaData(filePath string) Metadata {
	res := Metadata{
		filepath: filePath,
	}
	linkPath, symlinkErr := filepath.EvalSymlinks(filePath)
	if symlinkErr != nil {
		res.infoMsg = linkFileBrokenMsg
	} else {
		path := [2]string{keyPath, linkPath}
		res.data = append(res.data, path)
	}
	return res
}

func getMetaDataUnsorted(filePath string, metadataFocused bool, et *exiftool.Exiftool) Metadata {
	res := Metadata{
		filepath: filePath,
	}

	fileInfo, err := os.Lstat(filePath)
	if err != nil {
		res.infoMsg = fileStatErrorMsg
		return res
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return getSymLinkMetaData(filePath)
	}

	// Add basic metadata information irrespective of what is fetched from exiftool
	// Note : we prioritize these while sorting Metadata
	name := [2]string{keyName, fileInfo.Name()}
	size := [2]string{keySize, stringfunction.FormatFileSize(fileInfo.Size())}
	modifyDate := [2]string{keyDataModified, fileInfo.ModTime().String()}
	permissions := [2]string{keyPermissions, fileInfo.Mode().String()}
	ownerVal, groupVal := getOwnerAndGroup(fileInfo)
	owner := [2]string{keyOwner, ownerVal}
	group := [2]string{keyGroup, groupVal}

	if fileInfo.IsDir() && metadataFocused {
		// TODO : Calling dirSize() could be expensive for large directories, as it recursively
		// walks the entire tree. For now we have async approach of loading metadata,
		// and its only loaded when metadata panel is focussed.
		size = [2]string{keySize, stringfunction.FormatFileSize(utils.DirSize(filePath))}
	}

	res.data = append(res.data, name, size, modifyDate, permissions, owner, group)

	updateExiftoolMetadata(filePath, et, &res)

	return res
}

func updateExiftoolMetadata(filePath string, et *exiftool.Exiftool, res *Metadata) {
	if et == nil {
		return
	}
	fileInfos := et.ExtractMetadata(filePath)

	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			res.infoMsg = etFetchErrorMsg
			slog.Error("Error while return metadata function", "fileInfo", fileInfo, "error", fileInfo.Err)
			continue
		}
		for k, v := range fileInfo.Fields {
			res.data = append(res.data, [2]string{k, fmt.Sprintf("%v", v)})
		}
	}
}

func sortMetadata(meta [][2]string) {
	sort.SliceStable(meta, func(i, j int) bool {
		pi, iOkay := sortPriority[meta[i][0]]
		pj, jOkay := sortPriority[meta[j][0]]

		// Both are priority fields
		if iOkay && jOkay {
			return pi < pj
		}
		// i is a priority field, and j is not
		if iOkay {
			return true
		}

		// j is a priority field, and i is not
		if jOkay {
			return false
		}

		// None of them are priority fields, sort with name
		return meta[i][0] < meta[j][0]
	})
}
