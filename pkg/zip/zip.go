package zip

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExcludePaths contains paths that should be excluded from the zip file
var ExcludePaths = []string{
	".git",
	".github",
}

// CreateZip creates a zip file from a directory, excluding specified paths
func CreateZip(sourceDir, targetFile string) error {
	// Create the zip file
	zipFile, err := os.Create(targetFile)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	// Create a new zip writer
	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	// Walk through the source directory
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if it's a directory
		if info.IsDir() {
			return nil
		}

		// Check if the file should be excluded
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if shouldExclude(relPath) {
			return nil
		}

		// Create a new file in the zip
		file, err := writer.Create(relPath)
		if err != nil {
			return fmt.Errorf("failed to create file in zip: %w", err)
		}

		// Open the source file
		sourceFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open source file: %w", err)
		}
		defer sourceFile.Close()

		// Copy the file contents to the zip
		_, err = io.Copy(file, sourceFile)
		if err != nil {
			return fmt.Errorf("failed to copy file contents: %w", err)
		}

		return nil
	})

	return err
}

// shouldExclude checks if a path should be excluded from the zip file
func shouldExclude(path string) bool {
	for _, exclude := range ExcludePaths {
		if strings.HasPrefix(path, exclude) {
			return true
		}
	}
	return false
}

// GetFileSize returns the size of a file
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}
	return info.Size(), nil
}