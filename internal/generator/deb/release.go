package deb

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/utils"
)

// ReleaseFileInfo contains information about a file in the release
type ReleaseFileInfo struct {
	Path     string
	Checksum *utils.Checksum
}

// GenerateReleaseFile creates a Debian Release file
func GenerateReleaseFile(config *models.RepositoryConfig, files []ReleaseFileInfo) ([]byte, error) {
	var buf bytes.Buffer

	// Required fields
	fmt.Fprintf(&buf, "Origin: %s\n", config.Origin)
	fmt.Fprintf(&buf, "Label: %s\n", config.Label)
	fmt.Fprintf(&buf, "Suite: %s\n", config.Suite)
	fmt.Fprintf(&buf, "Codename: %s\n", config.Codename)
	fmt.Fprintf(&buf, "Architectures: %s\n", strings.Join(config.Arches, " "))
	fmt.Fprintf(&buf, "Components: %s\n", strings.Join(config.Components, " "))
	fmt.Fprintf(&buf, "Date: %s\n", time.Now().UTC().Format(time.RFC1123Z))

	// MD5Sum section
	buf.WriteString("MD5Sum:\n")
	for _, file := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", file.Checksum.MD5, file.Checksum.Size, file.Path)
	}

	// SHA1 section
	buf.WriteString("SHA1:\n")
	for _, file := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", file.Checksum.SHA1, file.Checksum.Size, file.Path)
	}

	// SHA256 section
	buf.WriteString("SHA256:\n")
	for _, file := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", file.Checksum.SHA256, file.Checksum.Size, file.Path)
	}

	// SHA512 section (optional but recommended)
	buf.WriteString("SHA512:\n")
	for _, file := range files {
		fmt.Fprintf(&buf, " %s %d %s\n", file.Checksum.SHA512, file.Checksum.Size, file.Path)
	}

	return buf.Bytes(), nil
}

// CalculateReleaseFileInfos calculates checksums for all metadata files
func CalculateReleaseFileInfos(basePath string, files []string) ([]ReleaseFileInfo, error) {
	var infos []ReleaseFileInfo

	for _, file := range files {
		fullPath := filepath.Join(basePath, file)
		checksum, err := utils.CalculateChecksums(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate checksum for %s: %w", file, err)
		}

		infos = append(infos, ReleaseFileInfo{
			Path:     file,
			Checksum: checksum,
		})
	}

	return infos, nil
}
