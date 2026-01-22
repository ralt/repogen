package apk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
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

	// Calculate control stream checksum for APKINDEX C: field
	// This is the SHA1 of the control gzip stream (second stream if signed, first if unsigned)
	controlChecksum, err := calculateControlChecksum(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate control checksum: %w", err)
	}

	// Set file information (keep full path for copying)
	pkg.Filename = path
	pkg.Size = checksums.Size
	pkg.MD5Sum = checksums.MD5
	pkg.SHA1Sum = checksums.SHA1
	pkg.SHA256Sum = checksums.SHA256
	pkg.SHA512Sum = checksums.SHA512
	pkg.ControlChecksum = controlChecksum

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

// calculateControlChecksum computes the SHA1 hash of the control gzip stream.
// For APK v2 format, the C: checksum in APKINDEX is the SHA1 of the control stream,
// which is the second gzip stream (after the signature stream, if present).
// This is base64-encoded with a "Q1" prefix in the APKINDEX.
func calculateControlChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// APK structure: [signature_stream] control_stream data_stream
	// If signed: 3 gzip streams (sig, control, data)
	// If unsigned: 2 gzip streams (control, data)
	offset := 0
	firstStreamEnd := findGzipStreamEnd(data, offset)
	if firstStreamEnd == -1 {
		return "", fmt.Errorf("could not find first gzip stream boundary")
	}

	// Check if first stream is a signature stream
	isSignature, err := isSignatureGzipStream(data[offset:firstStreamEnd])
	if err != nil {
		return "", err
	}

	var controlStart, controlEnd int
	if isSignature {
		// Signed package: control is the second stream
		controlStart = firstStreamEnd
		controlEnd = findGzipStreamEnd(data, controlStart)
		if controlEnd == -1 {
			return "", fmt.Errorf("could not find control gzip stream boundary")
		}
	} else {
		// Unsigned package: control is the first stream
		controlStart = offset
		controlEnd = firstStreamEnd
	}

	// Calculate SHA1 of the control stream raw bytes
	h := sha1.New()
	h.Write(data[controlStart:controlEnd])
	return hex.EncodeToString(h.Sum(nil)), nil
}

// findGzipStreamEnd finds the end offset of a gzip stream starting at the given offset.
// It searches for the next valid gzip header after the current stream.
func findGzipStreamEnd(data []byte, start int) int {
	if start >= len(data) {
		return -1
	}

	// Search for the next gzip header (0x1f 0x8b 0x08) after start
	// We start searching from start+1 to find the NEXT stream
	for i := start + 1; i < len(data)-2; i++ {
		if data[i] == 0x1f && data[i+1] == 0x8b && data[i+2] == 0x08 {
			// Verify this is a valid gzip stream by trying to create a reader
			testReader := bytes.NewReader(data[i:])
			testGz, err := gzip.NewReader(testReader)
			if err == nil {
				testGz.Close()
				return i
			}
		}
	}

	// No next stream found - this stream goes to the end
	return len(data)
}

// isSignatureGzipStream checks if a gzip stream contains a .SIGN.RSA.* file
func isSignatureGzipStream(data []byte) (bool, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		if strings.HasPrefix(header.Name, ".SIGN.RSA.") {
			return true, nil
		}
	}
	return false, nil
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
