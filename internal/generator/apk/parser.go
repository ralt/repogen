package apk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// ParseExistingMetadata reads APKINDEX.tar.gz files
func (g *Generator) ParseExistingMetadata(config *models.RepositoryConfig) ([]models.Package, error) {
	var allPackages []models.Package

	for _, arch := range config.Arches {
		archDir := filepath.Join(config.OutputDir, arch)
		apkindexPath := filepath.Join(archDir, "APKINDEX.tar.gz")

		packages, err := parseAPKINDEX(apkindexPath)
		if err != nil {
			// No existing metadata for this arch
			continue
		}

		allPackages = append(allPackages, packages...)
	}

	if len(allPackages) == 0 {
		return nil, fmt.Errorf("no existing APK metadata found in %s", config.OutputDir)
	}

	return allPackages, nil
}

func parseAPKINDEX(path string) ([]models.Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// Find APKINDEX file in tar
	var apkindexData []byte
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "APKINDEX" {
			apkindexData, err = io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	if len(apkindexData) == 0 {
		return nil, fmt.Errorf("APKINDEX not found in tar")
	}

	return parseAPKINDEXContent(apkindexData)
}

func parseAPKINDEXContent(data []byte) ([]models.Package, error) {
	var packages []models.Package
	var currentPkg *models.Package

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// Empty line = end of package
		if line == "" {
			if currentPkg != nil {
				packages = append(packages, *currentPkg)
				currentPkg = nil
			}
			continue
		}

		// Format: Letter:Value
		if len(line) < 2 || line[1] != ':' {
			continue
		}

		if currentPkg == nil {
			currentPkg = &models.Package{
				Metadata:     make(map[string]interface{}),
				Dependencies: []string{},
			}
		}

		field := line[0]
		value := line[2:]

		switch field {
		case 'C': // Checksum (Q1 prefix + base64 SHA1)
			if strings.HasPrefix(value, "Q1") {
				sha1Base64 := value[2:]
				sha1Bytes, _ := base64.StdEncoding.DecodeString(sha1Base64)
				currentPkg.SHA1Sum = hex.EncodeToString(sha1Bytes)
			}
		case 'P': // Package name
			currentPkg.Name = value
		case 'V': // Version
			currentPkg.Version = value
		case 'A': // Architecture
			currentPkg.Architecture = value
		case 'S': // Size
			size, _ := strconv.ParseInt(value, 10, 64)
			currentPkg.Size = size
		case 'I': // Installed size
			isize, _ := strconv.ParseInt(value, 10, 64)
			currentPkg.Metadata["installed_size"] = isize
		case 'T': // Description
			currentPkg.Description = value
		case 'U': // Homepage
			currentPkg.Homepage = value
		case 'L': // License
			currentPkg.License = value
		case 'D': // Dependencies (space-separated)
			currentPkg.Dependencies = strings.Fields(value)
		}
	}

	// Don't forget last package
	if currentPkg != nil {
		packages = append(packages, *currentPkg)
	}

	// Set filename for each package based on name-version.apk
	for i := range packages {
		pkg := &packages[i]
		// APK packages are named: name-version.apk
		pkg.Filename = fmt.Sprintf("%s-%s.apk", pkg.Name, pkg.Version)
	}

	return packages, scanner.Err()
}
