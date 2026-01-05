package deb

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/utils"
	"github.com/ulikunitz/xz"
)

// ParsePackage parses a .deb file and extracts metadata
func ParsePackage(path string) (*models.Package, error) {
	// Calculate checksums
	checksums, err := utils.CalculateChecksums(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksums: %w", err)
	}

	// Extract control file from the .deb
	control, err := extractControl(path)
	if err != nil {
		return nil, fmt.Errorf("failed to extract control: %w", err)
	}

	// Parse control file
	pkg, err := parseControl(control)
	if err != nil {
		return nil, fmt.Errorf("failed to parse control: %w", err)
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

// extractControl extracts the control file from a .deb package
func extractControl(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// .deb files are ar archives
	// Skip the first 8 bytes ("!<arch>\n")
	header := make([]byte, 8)
	if _, err := f.Read(header); err != nil {
		return nil, err
	}

	// Read ar archive entries
	for {
		// Read ar header (60 bytes)
		arHeader := make([]byte, 60)
		n, err := f.Read(arHeader)
		if err == io.EOF {
			break
		}
		if err != nil || n != 60 {
			return nil, fmt.Errorf("failed to read ar header")
		}

		// Parse filename (first 16 bytes, space-padded)
		// Also trim trailing slash that ar format may include
		filename := strings.TrimRight(strings.TrimSpace(string(arHeader[0:16])), "/")

		// Parse file size (bytes 48-58, decimal)
		sizeStr := strings.TrimSpace(string(arHeader[48:58]))
		var size int64
		fmt.Sscanf(sizeStr, "%d", &size)

		// Check if this is the control archive
		if strings.HasPrefix(filename, "control.tar") {
			// Read control archive data
			data := make([]byte, size)
			if _, err := io.ReadFull(f, data); err != nil {
				return nil, err
			}

			// Extract control file from control.tar
			return extractControlFromTar(data, filename)
		}

		// Skip this file's data
		if _, err := f.Seek(size, io.SeekCurrent); err != nil {
			return nil, err
		}

		// Align to 2-byte boundary
		if size%2 != 0 {
			f.Seek(1, io.SeekCurrent)
		}
	}

	return nil, fmt.Errorf("control.tar not found in package")
}

// extractControlFromTar extracts the control file from control.tar*
func extractControlFromTar(data []byte, filename string) ([]byte, error) {
	var tarReader *tar.Reader

	// Decompress based on extension
	if strings.HasSuffix(filename, ".gz") {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		tarReader = tar.NewReader(gr)
	} else if strings.HasSuffix(filename, ".xz") {
		xr, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		tarReader = tar.NewReader(xr)
	} else if strings.HasSuffix(filename, ".zst") {
		zr, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		tarReader = tar.NewReader(zr)
	} else {
		tarReader = tar.NewReader(bytes.NewReader(data))
	}

	// Find and read control file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "./control" || header.Name == "control" {
			return io.ReadAll(tarReader)
		}
	}

	return nil, fmt.Errorf("control file not found in control.tar")
}

// parseControl parses the Debian control file format
func parseControl(data []byte) (*models.Package, error) {
	pkg := &models.Package{
		Metadata: make(map[string]interface{}),
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var currentKey string
	var currentValue strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Handle continuation lines (start with space)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			currentValue.WriteString("\n")
			currentValue.WriteString(strings.TrimSpace(line))
			continue
		}

		// Save previous key-value pair
		if currentKey != "" {
			setValue(pkg, currentKey, currentValue.String())
		}

		// Parse new key-value pair
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			currentKey = strings.TrimSpace(parts[0])
			currentValue.Reset()
			if len(parts) > 1 {
				currentValue.WriteString(strings.TrimSpace(parts[1]))
			}
		}
	}

	// Save last key-value pair
	if currentKey != "" {
		setValue(pkg, currentKey, currentValue.String())
	}

	return pkg, scanner.Err()
}

// setValue sets a field in the Package based on the control file key
func setValue(pkg *models.Package, key, value string) {
	switch key {
	case "Package":
		pkg.Name = value
	case "Version":
		pkg.Version = value
	case "Architecture":
		pkg.Architecture = value
	case "Description":
		pkg.Description = value
	case "Maintainer":
		pkg.Maintainer = value
	case "Homepage":
		pkg.Homepage = value
	case "License":
		pkg.License = value
	case "Depends":
		// Parse dependencies (comma-separated)
		deps := strings.Split(value, ",")
		for _, dep := range deps {
			pkg.Dependencies = append(pkg.Dependencies, strings.TrimSpace(dep))
		}
	default:
		// Store other fields in metadata
		pkg.Metadata[key] = value
	}
}

// ParseExistingMetadata reads Packages files and returns existing packages
func (g *Generator) ParseExistingMetadata(config *models.RepositoryConfig) ([]models.Package, error) {
	var allPackages []models.Package

	// Iterate through all architectures and components
	for _, arch := range config.Arches {
		for _, comp := range config.Components {
			packagesPath := filepath.Join(
				config.OutputDir,
				"dists",
				config.Codename,
				comp,
				fmt.Sprintf("binary-%s", arch),
				"Packages",
			)

			// Try Packages first, fall back to Packages.gz
			packages, err := parsePackagesFile(packagesPath)
			if err != nil {
				packagesGzPath := packagesPath + ".gz"
				packages, err = parsePackagesGzFile(packagesGzPath)
				if err != nil {
					// No existing metadata for this arch/comp, skip
					continue
				}
			}

			allPackages = append(allPackages, packages...)
		}
	}

	if len(allPackages) == 0 {
		return nil, fmt.Errorf("no existing Debian metadata found in %s", config.OutputDir)
	}

	return allPackages, nil
}

func parsePackagesFile(path string) ([]models.Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parsePackagesReader(f)
}

func parsePackagesGzFile(path string) ([]models.Package, error) {
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

	return parsePackagesReader(gz)
}

func parsePackagesReader(r io.Reader) ([]models.Package, error) {
	var packages []models.Package
	var currentPkg *models.Package

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// Empty line = end of package entry
		if line == "" {
			if currentPkg != nil {
				packages = append(packages, *currentPkg)
				currentPkg = nil
			}
			continue
		}

		// Parse field: value
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		field := parts[0]
		value := parts[1]

		if currentPkg == nil {
			currentPkg = &models.Package{
				Metadata: make(map[string]interface{}),
			}
		}

		// Parse known fields
		switch field {
		case "Package":
			currentPkg.Name = value
		case "Version":
			currentPkg.Version = value
		case "Architecture":
			currentPkg.Architecture = value
		case "Filename":
			currentPkg.Filename = value
		case "Size":
			size, _ := strconv.ParseInt(value, 10, 64)
			currentPkg.Size = size
		case "MD5sum":
			currentPkg.MD5Sum = value
		case "SHA1":
			currentPkg.SHA1Sum = value
		case "SHA256":
			currentPkg.SHA256Sum = value
		case "SHA512":
			currentPkg.SHA512Sum = value
		case "Description":
			currentPkg.Description = value
		case "Maintainer":
			currentPkg.Maintainer = value
		case "Homepage":
			currentPkg.Homepage = value
		case "Depends":
			currentPkg.Dependencies = strings.Split(value, ", ")
		default:
			currentPkg.Metadata[field] = value
		}
	}

	// Don't forget last package
	if currentPkg != nil {
		packages = append(packages, *currentPkg)
	}

	return packages, scanner.Err()
}
