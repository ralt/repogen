package deb

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/ralt/repogen/internal/models"
)

// GeneratePackagesFile creates a Debian Packages file from package metadata
func GeneratePackagesFile(packages []models.Package) ([]byte, error) {
	var buf bytes.Buffer

	// Sort packages alphabetically by name
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})

	for _, pkg := range packages {
		// Required fields
		fmt.Fprintf(&buf, "Package: %s\n", pkg.Name)
		fmt.Fprintf(&buf, "Version: %s\n", pkg.Version)
		fmt.Fprintf(&buf, "Architecture: %s\n", pkg.Architecture)

		// File information
		fmt.Fprintf(&buf, "Filename: %s\n", pkg.Filename)
		fmt.Fprintf(&buf, "Size: %d\n", pkg.Size)
		fmt.Fprintf(&buf, "MD5sum: %s\n", pkg.MD5Sum)
		fmt.Fprintf(&buf, "SHA1: %s\n", pkg.SHA1Sum)
		fmt.Fprintf(&buf, "SHA256: %s\n", pkg.SHA256Sum)
		fmt.Fprintf(&buf, "SHA512: %s\n", pkg.SHA512Sum)

		// Optional fields
		if pkg.Maintainer != "" {
			fmt.Fprintf(&buf, "Maintainer: %s\n", pkg.Maintainer)
		}

		if pkg.Homepage != "" {
			fmt.Fprintf(&buf, "Homepage: %s\n", pkg.Homepage)
		}

		if pkg.Description != "" {
			fmt.Fprintf(&buf, "Description: %s\n", pkg.Description)
		}

		if len(pkg.Dependencies) > 0 {
			fmt.Fprintf(&buf, "Depends: %s\n", strings.Join(pkg.Dependencies, ", "))
		}

		// Add other metadata fields
		for key, value := range pkg.Metadata {
			// Skip fields we've already handled
			if key == "Package" || key == "Version" || key == "Architecture" ||
				key == "Maintainer" || key == "Homepage" || key == "Description" ||
				key == "Depends" {
				continue
			}
			fmt.Fprintf(&buf, "%s: %v\n", key, value)
		}

		// Blank line between packages
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}
