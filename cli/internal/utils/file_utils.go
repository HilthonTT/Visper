package utils

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

func WriteTomlData(filePath string, data any) error {
	tomlData, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("error encoding data: %w", err)
	}
	err = os.WriteFile(filePath, tomlData, 0o644)
	if err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}
	return nil
}

func LoadTomlFile(filePath, defaultData string, target any, fixFlag, ignoreMissingFields bool) error {
	_ = toml.Unmarshal([]byte(defaultData), target)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return &TomlLoadError{
			userMessage:  "config file doesn't exist",
			wrappedError: err,
		}
	}

	var rawData map[string]any
	err = toml.Unmarshal(data, &rawData)
	if err != nil {
		return &TomlLoadError{
			userMessage:  "error decoding TOML file",
			wrappedError: err,
			isFatal:      true,
		}
	}

	err = toml.Unmarshal(data, target)
	if err != nil {
		var decodeErr *toml.DecodeError
		if errors.As(err, &decodeErr) {
			row, col := decodeErr.Position()
			return &TomlLoadError{
				userMessage:  fmt.Sprintf("error in field at line %d column %d", row, col),
				wrappedError: decodeErr,
				isFatal:      true,
			}
		}
		return &TomlLoadError{
			userMessage:  "error unmarshalling data",
			wrappedError: err,
			isFatal:      true,
		}
	}

	// Override the default value if it exists default value to false
	if config, ok := target.(MissingFieldIgnorer); ok {
		ignoreMissingFields = config.GetIgnoreMissingFields()
	}

	// Check for missing fields
	targetType := reflect.TypeOf(target).Elem()
	missingFields := []string{}

	for i := range targetType.NumField() {
		field := targetType.Field(i)
		var fieldName string
		tag := field.Tag.Get("toml")

		if tag != "" {
			fieldName = strings.Split(tag, ",")[0]
		} else {
			fieldName = field.Name
		}

		if _, exists := rawData[fieldName]; !exists {
			missingFields = append(missingFields, fieldName)
		}
	}

	if len(missingFields) == 0 {
		return nil
	}

	if !fixFlag && ignoreMissingFields {
		// nil error if we dont wanna fix, and dont wanna print
		return nil
	}

	resultErr := &TomlLoadError{
		missingFields: true,
	}

	if !fixFlag {
		resultErr.userMessage = fmt.Sprintf("missing fields: %v", missingFields)
		return resultErr
	}

	// Start fixing
	return fixTomlFile(resultErr, filePath, target)
}

func fixTomlFile(resultErr *TomlLoadError, filePath string, target any) error {
	resultErr.isFatal = true
	// Create a unique backup of the current config file
	backupFile, err := os.CreateTemp(filepath.Dir(filePath), filepath.Base(filePath)+".bak-")
	if err != nil {
		resultErr.UpdateMessageAndError("failed to create backup file", err)
		return resultErr
	}

	backupPath := backupFile.Name()
	needsBackupFileRemoval := true

	defer func() {
		backupFile.Close()

		if needsBackupFileRemoval {
			// Remove backup in case of unsuccessful write
			if errRem := os.Remove(backupPath); errRem != nil {
				// Modify result Error
				resultErr.AddMessageAndError("warning: failed to remove backup file, backupPath : "+backupPath, errRem)
			}
		}
	}()

	// Copy the original file to the backup
	// Open it in read write mode
	origFile, err := os.OpenFile(filePath, os.O_RDWR, 0o644)
	if err != nil {
		resultErr.UpdateMessageAndError("failed to open original file for backup", err)
		return resultErr
	}
	defer origFile.Close()

	_, err = io.Copy(backupFile, origFile)
	if err != nil {
		resultErr.UpdateMessageAndError("failed to copy original file to backup", err)
		return resultErr
	}

	tomlData, err := toml.Marshal(target)
	if err != nil {
		resultErr.UpdateMessageAndError("failed to marshal config to TOML", err)
		return resultErr
	}

	_, err = origFile.WriteAt(tomlData, 0)
	if err != nil {
		resultErr.UpdateMessageAndError("failed to write TOML data to original file", err)
		return resultErr
	}

	// Fix done
	// Inform user about backup location
	resultErr.userMessage = "config file had issues. Its fixed successfully. Original backed up to : " + backupPath
	resultErr.isFatal = false
	// Do not remove backup; user may want to restore manually
	needsBackupFileRemoval = false

	return resultErr
}

func ResolveAbsPath(currentDir, path string) string {
	if !filepath.IsAbs(currentDir) {
		slog.Warn("currentDir is not absolute", "currentDir", currentDir)
	}

	if strings.HasPrefix(path, "~") {
		// We dont use variable.HomeDir here, as the util package cannot have dependency
		// on variable package
		path = strings.Replace(path, "~", xdg.Home, 1)
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(currentDir, path)
	}
	return filepath.Clean(path)
}
