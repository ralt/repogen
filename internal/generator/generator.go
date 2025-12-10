package generator

import (
	"context"

	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
)

// Generator interface for repository generators
type Generator interface {
	// Generate creates a repository structure from the provided packages
	Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error

	// ValidatePackages checks if packages are valid for this generator
	ValidatePackages(packages []models.Package) error

	// GetSupportedType returns the package type this generator supports
	GetSupportedType() scanner.PackageType
}
