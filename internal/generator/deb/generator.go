package deb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/signer"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
)

// Generator implements the generator.Generator interface for Debian repositories
type Generator struct {
	signer signer.Signer
}

// NewGenerator creates a new Debian generator
func NewGenerator(s signer.Signer) generator.Generator {
	return &Generator{
		signer: s,
	}
}

// Generate creates a Debian repository structure
func (g *Generator) Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error {
	logrus.Info("Generating Debian repository...")

	// Group packages by architecture
	archPackages := make(map[string][]models.Package)
	for _, pkg := range packages {
		arch := pkg.Architecture
		if arch == "" {
			arch = "amd64"
		}
		archPackages[arch] = append(archPackages[arch], pkg)
	}

	// Generate repository for each architecture
	for _, arch := range config.Arches {
		if err := g.generateForArch(ctx, config, arch, archPackages[arch]); err != nil {
			return fmt.Errorf("failed to generate for %s: %w", arch, err)
		}
	}

	// Generate Release file at repository root
	if err := g.generateRelease(config); err != nil {
		return fmt.Errorf("failed to generate Release: %w", err)
	}

	logrus.Info("Debian repository generated successfully")
	return nil
}

// generateForArch generates repository files for a specific architecture
func (g *Generator) generateForArch(ctx context.Context, config *models.RepositoryConfig, arch string, packages []models.Package) error {
	logrus.Infof("Generating for architecture: %s", arch)

	// Create directory structure
	// dists/{codename}/main/binary-{arch}/
	distsDir := filepath.Join(config.OutputDir, "dists", config.Codename, "main", fmt.Sprintf("binary-%s", arch))
	poolDir := filepath.Join(config.OutputDir, "pool", "main")

	if err := utils.EnsureDir(distsDir); err != nil {
		return err
	}
	if err := utils.EnsureDir(poolDir); err != nil {
		return err
	}

	// Copy packages to pool and update filenames
	for i := range packages {
		pkg := &packages[i]

		// Determine pool subdirectory (first letter of package name)
		firstLetter := string(pkg.Name[0])
		if firstLetter >= "a" && firstLetter <= "z" {
			// Use first letter
		} else {
			firstLetter = "0" // Use "0" for packages starting with numbers/special chars
		}

		// Create package directory: pool/main/{letter}/{name}/
		pkgDir := filepath.Join(poolDir, firstLetter, pkg.Name)
		if err := utils.EnsureDir(pkgDir); err != nil {
			return err
		}

		// Copy package file
		dstPath := filepath.Join(pkgDir, filepath.Base(pkg.Filename))
		if err := utils.CopyFile(pkg.Filename, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", pkg.Filename, err)
		}

		// Update filename to be relative to repository root
		relPath, err := filepath.Rel(config.OutputDir, dstPath)
		if err != nil {
			return err
		}
		pkg.Filename = relPath
	}

	// Generate Packages file
	packagesData, err := GeneratePackagesFile(packages)
	if err != nil {
		return fmt.Errorf("failed to generate Packages file: %w", err)
	}

	packagesPath := filepath.Join(distsDir, "Packages")
	if err := utils.WriteFile(packagesPath, packagesData, 0644); err != nil {
		return fmt.Errorf("failed to write Packages: %w", err)
	}

	// Compress Packages file
	packagesGz, err := utils.GzipCompress(packagesData)
	if err != nil {
		return fmt.Errorf("failed to compress Packages: %w", err)
	}

	packagesGzPath := filepath.Join(distsDir, "Packages.gz")
	if err := utils.WriteFile(packagesGzPath, packagesGz, 0644); err != nil {
		return fmt.Errorf("failed to write Packages.gz: %w", err)
	}

	logrus.Infof("Generated Packages files for %s (%d packages)", arch, len(packages))
	return nil
}

// generateRelease generates the Release, InRelease, and Release.gpg files
func (g *Generator) generateRelease(config *models.RepositoryConfig) error {
	logrus.Info("Generating Release file...")

	distsDir := filepath.Join(config.OutputDir, "dists", config.Codename)

	// Find all Packages files
	var metadataFiles []string
	for _, arch := range config.Arches {
		for _, comp := range config.Components {
			binDir := fmt.Sprintf("%s/binary-%s", comp, arch)

			// Add Packages
			packagesPath := filepath.Join(binDir, "Packages")
			metadataFiles = append(metadataFiles, packagesPath)

			// Add Packages.gz
			packagesGzPath := filepath.Join(binDir, "Packages.gz")
			metadataFiles = append(metadataFiles, packagesGzPath)
		}
	}

	// Calculate checksums for metadata files
	fileInfos, err := CalculateReleaseFileInfos(distsDir, metadataFiles)
	if err != nil {
		return err
	}

	// Generate Release file
	releaseData, err := GenerateReleaseFile(config, fileInfos)
	if err != nil {
		return fmt.Errorf("failed to generate Release file: %w", err)
	}

	releasePath := filepath.Join(distsDir, "Release")
	if err := utils.WriteFile(releasePath, releaseData, 0644); err != nil {
		return fmt.Errorf("failed to write Release: %w", err)
	}

	// Sign if signer is available
	if g.signer != nil {
		// Create InRelease (cleartext signed)
		inReleaseData, err := g.signer.SignCleartext(releaseData)
		if err != nil {
			return fmt.Errorf("failed to sign InRelease: %w", err)
		}

		inReleasePath := filepath.Join(distsDir, "InRelease")
		if err := utils.WriteFile(inReleasePath, inReleaseData, 0644); err != nil {
			return fmt.Errorf("failed to write InRelease: %w", err)
		}

		// Create Release.gpg (detached signature)
		releaseGpg, err := g.signer.SignDetached(releaseData)
		if err != nil {
			return fmt.Errorf("failed to create Release.gpg: %w", err)
		}

		releaseGpgPath := filepath.Join(distsDir, "Release.gpg")
		if err := utils.WriteFile(releaseGpgPath, releaseGpg, 0644); err != nil {
			return fmt.Errorf("failed to write Release.gpg: %w", err)
		}

		logrus.Info("Release file signed successfully")
	} else {
		// For unsigned repositories, create InRelease with Release content
		// This allows modern apt (especially Debian Trixie) to work with [trusted=yes]
		inReleasePath := filepath.Join(distsDir, "InRelease")
		if err := utils.WriteFile(inReleasePath, releaseData, 0644); err != nil {
			return fmt.Errorf("failed to write InRelease: %w", err)
		}

		logrus.Warn("No signer configured, repository will be unsigned")
		logrus.Info("Generated InRelease file for compatibility with modern apt")
	}

	return nil
}

// ValidatePackages checks if packages are valid Debian packages
func (g *Generator) ValidatePackages(packages []models.Package) error {
	for _, pkg := range packages {
		if pkg.Name == "" {
			return fmt.Errorf("package missing name: %s", pkg.Filename)
		}
		if pkg.Version == "" {
			return fmt.Errorf("package %s missing version", pkg.Name)
		}
		if pkg.Architecture == "" {
			return fmt.Errorf("package %s missing architecture", pkg.Name)
		}
		if !strings.HasSuffix(pkg.Filename, ".deb") {
			return fmt.Errorf("package %s is not a .deb file", pkg.Name)
		}
	}
	return nil
}

// GetSupportedType returns the package type this generator supports
func (g *Generator) GetSupportedType() scanner.PackageType {
	return scanner.TypeDeb
}
