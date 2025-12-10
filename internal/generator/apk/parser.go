package apk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/utils"
)

// ParsePackage parses an APK file and extracts metadata
func ParsePackage(path string) (*models.Package, error) {
	// Calculate checksums
	checksums, err := utils.CalculateChecksums(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksums: %w", err)
	}

	// Extract .PKGINFO from the APK
	pkginfo, err := extractPKGINFO(path)
	if err != nil {
		return nil, fmt.Errorf("failed to extract PKGINFO: %w", err)
	}

	// Parse PKGINFO
	pkg, err := parsePKGINFO(pkginfo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PKGINFO: %w", err)
	}

	// Set file information (keep full path for copying)
	pkg.Filename = path
	pkg.Size = checksums.Size
	pkg.MD5Sum = checksums.MD5
	pkg.SHA1Sum = checksums.SHA1
	pkg.SHA256Sum = checksums.SHA256
	pkg.SHA512Sum = checksums.SHA512

	return pkg, nil
}

// extractPKGINFO extracts the .PKGINFO file from an APK package
func extractPKGINFO(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// APK files are gzipped tar archives
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Find .PKGINFO file
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == ".PKGINFO" {
			return io.ReadAll(tr)
		}
	}

	return nil, fmt.Errorf(".PKGINFO not found in APK")
}

// parsePKGINFO parses the Alpine PKGINFO format
func parsePKGINFO(data []byte) (*models.Package, error) {
	pkg := &models.Package{
		Metadata: make(map[string]interface{}),
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, " = ", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "pkgname":
			pkg.Name = value
		case "pkgver":
			pkg.Version = value
		case "arch":
			pkg.Architecture = value
		case "pkgdesc":
			pkg.Description = value
		case "url":
			pkg.Homepage = value
		case "license":
			pkg.License = value
		case "depend":
			pkg.Dependencies = append(pkg.Dependencies, value)
		case "size":
			if size, err := strconv.ParseInt(value, 10, 64); err == nil {
				pkg.Metadata["installed_size"] = size
			}
		default:
			pkg.Metadata[key] = value
		}
	}

	return pkg, scanner.Err()
}
