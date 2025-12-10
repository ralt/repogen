package rpm

import (
	"fmt"
	"os"
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
