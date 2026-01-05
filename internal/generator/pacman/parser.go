package pacman

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/utils"
	"github.com/ulikunitz/xz"
)

// ParsePackage parses a Pacman package file and extracts metadata
func ParsePackage(path string) (*models.Package, error) {
	// Calculate checksums
	checksums, err := utils.CalculateChecksums(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksums: %w", err)
	}

	// Extract .PKGINFO file
	pkginfo, err := extractPKGINFO(path)
	if err != nil {
		return nil, fmt.Errorf("failed to extract .PKGINFO: %w", err)
	}

	// Parse .PKGINFO
	pkg, err := parsePKGINFO(pkginfo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .PKGINFO: %w", err)
	}

	// Set file information
	pkg.Filename = path
	pkg.Size = checksums.Size
	pkg.MD5Sum = checksums.MD5
	pkg.SHA1Sum = checksums.SHA1
	pkg.SHA256Sum = checksums.SHA256
	pkg.SHA512Sum = checksums.SHA512

	return pkg, nil
}

// extractPKGINFO extracts the .PKGINFO file from a Pacman package
func extractPKGINFO(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Detect compression from extension
	var tarReader *tar.Reader

	if strings.HasSuffix(path, ".pkg.tar.zst") {
		zr, err := zstd.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		tarReader = tar.NewReader(zr)
	} else if strings.HasSuffix(path, ".pkg.tar.xz") {
		xr, err := xz.NewReader(f)
		if err != nil {
			return nil, err
		}
		tarReader = tar.NewReader(xr)
	} else if strings.HasSuffix(path, ".pkg.tar.gz") {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		tarReader = tar.NewReader(gr)
	} else if strings.HasSuffix(path, ".pkg.tar") {
		tarReader = tar.NewReader(f)
	} else {
		return nil, fmt.Errorf("unsupported package format: %s", filepath.Base(path))
	}

	// Find .PKGINFO
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == ".PKGINFO" {
			return io.ReadAll(tarReader)
		}
	}

	return nil, fmt.Errorf(".PKGINFO not found in package")
}

// parsePKGINFO parses the .PKGINFO file content
func parsePKGINFO(data []byte) (*models.Package, error) {
	pkg := &models.Package{
		Metadata: make(map[string]interface{}),
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key = value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Map fields to Package struct
		switch key {
		case "pkgname":
			pkg.Name = value
		case "pkgver":
			pkg.Version = value
		case "pkgdesc":
			pkg.Description = value
		case "url":
			pkg.Homepage = value
		case "license":
			pkg.License = value
		case "arch":
			pkg.Architecture = value
		case "packager":
			pkg.Maintainer = value
		case "depend":
			pkg.Dependencies = append(pkg.Dependencies, value)
		case "conflict":
			pkg.Conflicts = append(pkg.Conflicts, value)
		case "group":
			pkg.Groups = append(pkg.Groups, value)
		case "builddate":
			pkg.Metadata["BuildDate"] = value
		case "size":
			pkg.Metadata["InstalledSize"] = value
		default:
			// Store other fields in metadata
			pkg.Metadata[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return pkg, nil
}

// ParseExistingMetadata reads .db.tar.zst database files
func (g *Generator) ParseExistingMetadata(config *models.RepositoryConfig) ([]models.Package, error) {
	var allPackages []models.Package

	// Pacman repos are organized by arch
	for _, arch := range config.Arches {
		archDir := filepath.Join(config.OutputDir, arch)

		// Find database files (pattern: *.db.tar.zst or *.db)
		pattern := filepath.Join(archDir, "*.db.tar.zst")
		dbFiles, err := filepath.Glob(pattern)
		if err != nil || len(dbFiles) == 0 {
			// Try .db.tar.xz extension
			pattern = filepath.Join(archDir, "*.db.tar.xz")
			dbFiles, err = filepath.Glob(pattern)
			if err != nil || len(dbFiles) == 0 {
				// Try .db.tar.gz extension
				pattern = filepath.Join(archDir, "*.db.tar.gz")
				dbFiles, err = filepath.Glob(pattern)
				if err != nil || len(dbFiles) == 0 {
					// Try .db extension (might be symlink or uncompressed)
					pattern = filepath.Join(archDir, "*.db")
					dbFiles, err = filepath.Glob(pattern)
					if err != nil || len(dbFiles) == 0 {
						continue
					}
				}
			}
		}

		// Parse first database file found
		packages, err := parsePacmanDB(dbFiles[0])
		if err != nil {
			continue
		}

		allPackages = append(allPackages, packages...)
	}

	if len(allPackages) == 0 {
		return nil, fmt.Errorf("no existing Pacman metadata found in %s", config.OutputDir)
	}

	return allPackages, nil
}

func parsePacmanDB(dbPath string) ([]models.Package, error) {
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Detect compression from extension
	var tarReader *tar.Reader

	if strings.HasSuffix(dbPath, ".db.tar.zst") || strings.HasSuffix(dbPath, ".db.tar") {
		// Try zstd first
		zr, err := zstd.NewReader(f)
		if err != nil {
			// If zstd fails and it's .db.tar, try uncompressed
			if strings.HasSuffix(dbPath, ".db.tar") {
				f.Seek(0, 0)
				tarReader = tar.NewReader(f)
			} else {
				return nil, err
			}
		} else {
			defer zr.Close()
			tarReader = tar.NewReader(zr)
		}
	} else if strings.HasSuffix(dbPath, ".db.tar.xz") {
		xr, err := xz.NewReader(f)
		if err != nil {
			return nil, err
		}
		tarReader = tar.NewReader(xr)
	} else if strings.HasSuffix(dbPath, ".db.tar.gz") {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		tarReader = tar.NewReader(gr)
	} else {
		// Assume it's a symlink or uncompressed tar
		tarReader = tar.NewReader(f)
	}

	var packages []models.Package

	// Read tar archive - each package has a directory with desc file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Each package has a directory with desc file
		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, "/desc") {
			// Read desc file
			descData, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}

			pkg, err := parseDescFile(descData)
			if err != nil {
				continue
			}

			packages = append(packages, *pkg)
		}
	}

	return packages, nil
}

func parseDescFile(data []byte) (*models.Package, error) {
	pkg := &models.Package{
		Metadata:     make(map[string]interface{}),
		Dependencies: []string{},
		Conflicts:    []string{},
		Groups:       []string{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var currentField string

	for scanner.Scan() {
		line := scanner.Text()

		// Field marker: %FIELDNAME%
		if strings.HasPrefix(line, "%") && strings.HasSuffix(line, "%") {
			currentField = strings.Trim(line, "%")
			continue
		}

		// Empty line
		if line == "" {
			currentField = ""
			continue
		}

		// Value for current field
		switch currentField {
		case "FILENAME":
			pkg.Filename = line
		case "NAME":
			pkg.Name = line
		case "VERSION":
			pkg.Version = line
		case "DESC":
			pkg.Description = line
		case "CSIZE":
			size := int64(0)
			fmt.Sscanf(line, "%d", &size)
			pkg.Size = size
		case "MD5SUM":
			pkg.MD5Sum = line
		case "SHA256SUM":
			pkg.SHA256Sum = line
		case "ARCH":
			pkg.Architecture = line
		case "PACKAGER":
			pkg.Maintainer = line
		case "URL":
			pkg.Homepage = line
		case "LICENSE":
			pkg.License = line
		case "BUILDDATE":
			pkg.Metadata["BuildDate"] = line
		case "ISIZE":
			pkg.Metadata["InstalledSize"] = line
		case "DEPENDS":
			pkg.Dependencies = append(pkg.Dependencies, line)
		case "CONFLICTS":
			pkg.Conflicts = append(pkg.Conflicts, line)
		case "GROUPS":
			pkg.Groups = append(pkg.Groups, line)
		}
	}

	return pkg, nil
}
