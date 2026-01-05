package utils

import (
	"fmt"

	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
)

// PackageIdentity returns a unique identifier for a package based on format
func PackageIdentity(pkg models.Package, pkgType scanner.PackageType) string {
	switch pkgType {
	case scanner.TypeDeb, scanner.TypeApk, scanner.TypePacman:
		return fmt.Sprintf("%s:%s:%s", pkg.Name, pkg.Version, pkg.Architecture)
	case scanner.TypeRpm:
		release := "1"
		if r, ok := pkg.Metadata["Release"].(string); ok {
			release = r
		}
		return fmt.Sprintf("%s:%s:%s:%s", pkg.Name, pkg.Version, release, pkg.Architecture)
	case scanner.TypeHomebrewBottle:
		return fmt.Sprintf("%s:%s", pkg.Name, pkg.Version)
	default:
		return fmt.Sprintf("%s:%s", pkg.Name, pkg.Version)
	}
}

// DetectConflicts returns packages from newPackages that conflict with existing
func DetectConflicts(existing, newPackages []models.Package, pkgType scanner.PackageType) []models.Package {
	existingMap := make(map[string]bool)
	for _, pkg := range existing {
		existingMap[PackageIdentity(pkg, pkgType)] = true
	}

	var conflicts []models.Package
	for _, pkg := range newPackages {
		if existingMap[PackageIdentity(pkg, pkgType)] {
			conflicts = append(conflicts, pkg)
		}
	}
	return conflicts
}
