package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ralt/repogen/internal/models"
)

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	// Create destination directory if it doesn't exist
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Sync to disk
	return dstFile.Sync()
}

// WriteFile writes data to a file, creating directories as needed
func WriteFile(path string, data []byte, perm os.FileMode) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, perm)
}

// EnsureDir ensures a directory exists, creating it if necessary
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// ShouldCopyPackage determines if a package file needs to be copied.
// It handles both new packages (from input directory) and existing packages (from metadata).
// Returns: (srcPath, dstPath, needsCopy, error)
func ShouldCopyPackage(pkg *models.Package, dstPath, outputDir string) (string, string, bool, error) {
	// Normalize paths
	dstPath = filepath.Clean(dstPath)
	outputDir = filepath.Clean(outputDir)

	// Strategy: Try to find the source file in two possible locations:
	// 1. At pkg.Filename as-is (new package from input directory)
	// 2. At outputDir + pkg.Filename (existing package from previous generation)

	// First, try pkg.Filename as-is (new package from input)
	srcPath := filepath.Clean(pkg.Filename)
	srcInfo, err := os.Stat(srcPath)
	isNewPackage := (err == nil) // If file exists at this path, it's a new package

	if !isNewPackage {
		// File doesn't exist at pkg.Filename, so try as existing package
		// Existing packages have paths relative to output directory
		srcPath = filepath.Clean(filepath.Join(outputDir, pkg.Filename))
		srcInfo, err = os.Stat(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist locally - this is OK for existing packages in S3
				return srcPath, dstPath, false, nil
			}
			return srcPath, dstPath, false, fmt.Errorf("cannot stat source: %w", err)
		}
	}

	// Source file exists - now check if we need to copy it

	// Same path = no copy needed
	if srcPath == dstPath {
		return srcPath, dstPath, false, nil
	}

	// Check destination existence
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Destination doesn't exist, need to copy
			return srcPath, dstPath, true, nil
		}
		return srcPath, dstPath, false, fmt.Errorf("cannot stat destination: %w", err)
	}

	// Both source and destination exist - compare to see if copy needed

	// Different sizes = need copy
	if srcInfo.Size() != dstInfo.Size() {
		return srcPath, dstPath, true, nil
	}

	// Same size - compare checksums if available
	if pkg.SHA256Sum != "" {
		dstChecksums, err := CalculateChecksums(dstPath)
		if err != nil {
			// Can't calculate checksums, copy to be safe
			return srcPath, dstPath, true, nil
		}
		if pkg.SHA256Sum != dstChecksums.SHA256 {
			// Different checksums = need copy
			return srcPath, dstPath, true, nil
		}
	}

	// Files appear to be the same, skip copy
	return srcPath, dstPath, false, nil
}
