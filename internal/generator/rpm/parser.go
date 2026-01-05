package rpm

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sassoftware/go-rpmutils"
)

// ParsePackage parses an RPM file and extracts metadata
func ParsePackage(path string) (*models.Package, error) {
	// Calculate checksums
	checksums, err := utils.CalculateChecksums(path)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksums: %w", err)
	}

	// Open RPM file
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read RPM header
	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read RPM: %w", err)
	}

	// Extract metadata
	pkg := &models.Package{
		Name:         getStringTag(rpm, rpmutils.NAME),
		Version:      getStringTag(rpm, rpmutils.VERSION),
		Architecture: getStringTag(rpm, rpmutils.ARCH),
		Description:  getStringTag(rpm, rpmutils.SUMMARY),
		Maintainer:   getStringTag(rpm, rpmutils.PACKAGER),
		Homepage:     getStringTag(rpm, rpmutils.URL),
		License:      getStringTag(rpm, rpmutils.LICENSE),
		Dependencies: getStringSliceTag(rpm, rpmutils.REQUIRENAME),
		Metadata:     make(map[string]interface{}),
	}

	// Set file information (keep full path for copying)
	pkg.Filename = path
	pkg.Size = checksums.Size
	pkg.MD5Sum = checksums.MD5
	pkg.SHA1Sum = checksums.SHA1
	pkg.SHA256Sum = checksums.SHA256
	pkg.SHA512Sum = checksums.SHA512

	// Add additional metadata
	pkg.Metadata["Release"] = getStringTag(rpm, rpmutils.RELEASE)
	pkg.Metadata["Group"] = getStringTag(rpm, rpmutils.GROUP)
	pkg.Metadata["BuildTime"] = getIntTag(rpm, rpmutils.BUILDTIME)
	pkg.Metadata["DistroVersion"] = getDistroVersion(rpm)

	return pkg, nil
}

// getStringTag safely gets a string tag from RPM
func getStringTag(rpm *rpmutils.Rpm, tag int) string {
	val, err := rpm.Header.Get(tag)
	if err != nil {
		return ""
	}

	// Handle different types that might be returned
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	default:
		// Try to convert to string using fmt
		return fmt.Sprintf("%v", v)
	}

	return ""
}

// getIntTag safely gets an integer tag from RPM
func getIntTag(rpm *rpmutils.Rpm, tag int) int64 {
	val, err := rpm.Header.Get(tag)
	if err != nil {
		return 0
	}
	if i, ok := val.(int); ok {
		return int64(i)
	}
	if i64, ok := val.(int64); ok {
		return i64
	}
	return 0
}

// getStringSliceTag safely gets a string slice tag from RPM
func getStringSliceTag(rpm *rpmutils.Rpm, tag int) []string {
	val, err := rpm.Header.Get(tag)
	if err != nil {
		return nil
	}
	if slice, ok := val.([]string); ok {
		// Filter out empty strings
		var result []string
		for _, s := range slice {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// getDistroVersion extracts the distribution version from RPM metadata
// It parses patterns like fc40 -> 40, el8 -> 8, el9 -> 9
func getDistroVersion(rpm *rpmutils.Rpm) string {
	// Try DISTURL first (1123 is the tag number for DISTURL)
	disturl := getStringTag(rpm, 1123)
	if disturl != "" {
		if version := parseVersionFromDistro(disturl); version != "" {
			return version
		}
	}

	// Try DISTRIBUTION tag (1010 is the tag number for DISTRIBUTION)
	dist := getStringTag(rpm, 1010)
	if dist != "" {
		if version := parseVersionFromDistro(dist); version != "" {
			return version
		}
	}

	// Try DISTTAG (1155)
	disttag := getStringTag(rpm, 1155)
	if disttag != "" {
		if version := parseVersionFromDistro(disttag); version != "" {
			return version
		}
	}

	return ""
}

// parseVersionFromDistro parses version from distribution strings
// Handles patterns: fc40 -> 40, el8 -> 8, .el9 -> 9, etc.
func parseVersionFromDistro(distro string) string {
	// Common patterns for Fedora, RHEL, CentOS
	patterns := []string{
		`fc(\d+)`,     // Fedora: fc40, fc39
		`\.fc(\d+)`,   // Fedora with dot: .fc40
		`el(\d+)`,     // RHEL/CentOS: el8, el9
		`\.el(\d+)`,   // RHEL/CentOS with dot: .el8, .el9
		`\.c(\d+)`,    // CentOS: .c8, .c9
		`fedora(\d+)`, // Fedora: fedora40
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(distro); len(matches) > 1 {
			return matches[1]
		}
	}

	// If no pattern matched, try to extract the first sequence of digits
	re := regexp.MustCompile(`(\d+)`)
	if matches := re.FindStringSubmatch(distro); len(matches) > 0 {
		return matches[1]
	}

	return ""
}

// ParseExistingMetadata reads primary.xml from repodata
func (g *Generator) ParseExistingMetadata(config *models.RepositoryConfig) ([]models.Package, error) {
	var allPackages []models.Package

	// RPM repos are organized by version/arch
	// We need to scan all version/arch combinations
	entries, err := os.ReadDir(config.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read output directory: %w", err)
	}

	for _, versionEntry := range entries {
		if !versionEntry.IsDir() {
			continue
		}

		versionDir := filepath.Join(config.OutputDir, versionEntry.Name())
		archEntries, err := os.ReadDir(versionDir)
		if err != nil {
			continue
		}

		for _, archEntry := range archEntries {
			if !archEntry.IsDir() {
				continue
			}

			archDir := filepath.Join(versionDir, archEntry.Name())
			packages, err := parsePrimaryXML(archDir)
			if err != nil {
				// No metadata in this arch dir, skip
				continue
			}

			allPackages = append(allPackages, packages...)
		}
	}

	if len(allPackages) == 0 {
		return nil, fmt.Errorf("no existing RPM metadata found in %s", config.OutputDir)
	}

	return allPackages, nil
}

func parsePrimaryXML(archDir string) ([]models.Package, error) {
	// Read repomd.xml to find primary.xml location
	repomdPath := filepath.Join(archDir, "repodata", "repomd.xml")
	repomdData, err := os.ReadFile(repomdPath)
	if err != nil {
		return nil, err
	}

	var repomdDoc repomd
	if err := xml.Unmarshal(repomdData, &repomdDoc); err != nil {
		return nil, err
	}

	// Find primary.xml location
	var primaryLocation string
	for _, data := range repomdDoc.Data {
		if data.Type == "primary" {
			primaryLocation = data.Location.Href
			break
		}
	}

	if primaryLocation == "" {
		return nil, fmt.Errorf("primary.xml not found in repomd.xml")
	}

	// Read and decompress primary.xml.gz
	primaryPath := filepath.Join(archDir, primaryLocation)
	f, err := os.Open(primaryPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}

	// Parse XML
	var meta metadata
	if err := xml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	// Convert to models.Package
	var packages []models.Package
	for _, xmlPkg := range meta.Packages {
		pkg := models.Package{
			Name:         xmlPkg.Name,
			Version:      xmlPkg.Version.Ver,
			Architecture: xmlPkg.Arch,
			Description:  xmlPkg.Summary,
			Maintainer:   xmlPkg.Packager,
			Homepage:     xmlPkg.URL,
			License:      xmlPkg.Format.License,
			Filename:     xmlPkg.Location.Href,
			Size:         xmlPkg.Size.Package,
			SHA256Sum:    xmlPkg.Checksum.Value,
			Metadata: map[string]interface{}{
				"Release":   xmlPkg.Version.Rel,
				"BuildTime": xmlPkg.Time.Build,
				"Group":     xmlPkg.Format.Group,
			},
		}
		packages = append(packages, pkg)
	}

	return packages, nil
}
