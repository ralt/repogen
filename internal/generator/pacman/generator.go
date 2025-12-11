package pacman

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/signer"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
)

// Generator implements the generator.Generator interface for Pacman repositories
type Generator struct {
	signer signer.Signer
}

// NewGenerator creates a new Pacman generator
func NewGenerator(s signer.Signer) generator.Generator {
	return &Generator{
		signer: s,
	}
}

// Generate creates a Pacman repository structure
func (g *Generator) Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error {
	logrus.Info("Generating Pacman repository...")

	// Group packages by architecture
	archPackages := make(map[string][]models.Package)

	for _, pkg := range packages {
		arch := pkg.Architecture
		if arch == "" {
			arch = "x86_64" // default architecture
		}
		archPackages[arch] = append(archPackages[arch], pkg)
	}

	// Generate repository for each architecture
	for arch, pkgs := range archPackages {
		if err := g.generateForArch(ctx, config, arch, pkgs); err != nil {
			return fmt.Errorf("failed to generate for %s: %w", arch, err)
		}
	}

	if g.signer != nil {
		logrus.Info("Repository signed successfully")
	}

	logrus.Infof("Pacman repository generated successfully (%d packages)", len(packages))
	return nil
}

// generateForArch generates repository for a specific architecture
func (g *Generator) generateForArch(ctx context.Context, config *models.RepositoryConfig, arch string, packages []models.Package) error {
	logrus.Infof("Generating for architecture: %s", arch)

	// Create directory structure: OutputDir/arch/
	archDir := filepath.Join(config.OutputDir, arch)
	if err := utils.EnsureDir(archDir); err != nil {
		return err
	}

	// Copy packages to arch directory
	for i, pkg := range packages {
		srcPath := pkg.Filename
		dstPath := filepath.Join(archDir, filepath.Base(pkg.Filename))

		if err := utils.CopyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy package: %w", err)
		}

		// Update filename to just the basename for database
		packages[i].Filename = filepath.Base(pkg.Filename)
	}

	// Generate database name from repo-name, origin, or default
	dbName := "custom"
	if config.RepoName != "" {
		dbName = sanitizeRepoName(config.RepoName)
	} else if config.Origin != "" {
		dbName = sanitizeRepoName(config.Origin)
	}

	// Generate database
	dbData, err := g.generateDatabase(config, packages)
	if err != nil {
		return fmt.Errorf("failed to generate database: %w", err)
	}

	// Write database file to arch directory without arch suffix
	dbPath := filepath.Join(archDir, fmt.Sprintf("%s.db.tar.zst", dbName))
	if err := utils.WriteFile(dbPath, dbData, 0644); err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	// Sign database if signer available
	if g.signer != nil {
		signature, err := g.signer.SignDetached(dbData)
		if err != nil {
			return fmt.Errorf("failed to sign database: %w", err)
		}

		sigPath := fmt.Sprintf("%s.sig", dbPath)
		if err := utils.WriteFile(sigPath, signature, 0644); err != nil {
			return fmt.Errorf("failed to write database signature: %w", err)
		}

		// Sign each package file
		for _, pkg := range packages {
			pkgPath := filepath.Join(archDir, pkg.Filename)
			pkgData, err := os.ReadFile(pkgPath)
			if err != nil {
				return fmt.Errorf("failed to read package %s: %w", pkg.Filename, err)
			}

			pkgSig, err := g.signer.SignDetached(pkgData)
			if err != nil {
				return fmt.Errorf("failed to sign package %s: %w", pkg.Filename, err)
			}

			pkgSigPath := fmt.Sprintf("%s.sig", pkgPath)
			if err := utils.WriteFile(pkgSigPath, pkgSig, 0644); err != nil {
				return fmt.Errorf("failed to write package signature: %w", err)
			}
		}
	}

	logrus.Infof("Generated repository for %s (%d packages)", arch, len(packages))
	return nil
}

// generateDatabase creates the Pacman database (.db.tar.zst)
func (g *Generator) generateDatabase(config *models.RepositoryConfig, packages []models.Package) ([]byte, error) {
	// Create in-memory tar archive
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	for _, pkg := range packages {
		// Generate desc content
		descContent, err := generateDescFile(pkg)
		if err != nil {
			return nil, fmt.Errorf("failed to generate desc for %s: %w", pkg.Name, err)
		}

		// Create directory entry
		dirName := fmt.Sprintf("%s-%s/", pkg.Name, pkg.Version)
		err = tw.WriteHeader(&tar.Header{
			Name:     dirName,
			Mode:     0755,
			Typeflag: tar.TypeDir,
		})
		if err != nil {
			return nil, err
		}

		// Add desc file
		descPath := dirName + "desc"
		err = tw.WriteHeader(&tar.Header{
			Name: descPath,
			Mode: 0644,
			Size: int64(len(descContent)),
		})
		if err != nil {
			return nil, err
		}

		_, err = tw.Write(descContent)
		if err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	// Compress with zstd
	var compressedBuf bytes.Buffer
	zw, err := zstd.NewWriter(&compressedBuf)
	if err != nil {
		return nil, err
	}

	_, err = zw.Write(tarBuf.Bytes())
	if err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return compressedBuf.Bytes(), nil
}

// generateDescFile creates the desc file content for a package
func generateDescFile(pkg models.Package) ([]byte, error) {
	var buf bytes.Buffer

	// Write a field to the buffer
	writeField := func(name, value string) {
		if value != "" {
			buf.WriteString(fmt.Sprintf("%%%s%%\n%s\n\n", name, value))
		}
	}

	// Required fields
	writeField("FILENAME", pkg.Filename)
	writeField("NAME", pkg.Name)
	writeField("VERSION", pkg.Version)
	writeField("DESC", pkg.Description)

	// File sizes
	writeField("CSIZE", fmt.Sprintf("%d", pkg.Size))
	if installedSize, ok := pkg.Metadata["InstalledSize"].(string); ok && installedSize != "" {
		writeField("ISIZE", installedSize)
	}

	// Checksums
	writeField("MD5SUM", pkg.MD5Sum)
	writeField("SHA256SUM", pkg.SHA256Sum)

	// Architecture
	writeField("ARCH", pkg.Architecture)

	// Optional fields
	if buildDate, ok := pkg.Metadata["BuildDate"].(string); ok && buildDate != "" {
		writeField("BUILDDATE", buildDate)
	}
	writeField("PACKAGER", pkg.Maintainer)
	writeField("URL", pkg.Homepage)
	writeField("LICENSE", pkg.License)

	// Dependencies
	if len(pkg.Dependencies) > 0 {
		buf.WriteString("%DEPENDS%\n")
		for _, dep := range pkg.Dependencies {
			buf.WriteString(fmt.Sprintf("%s\n", dep))
		}
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}

// sanitizeRepoName sanitizes a repository name for use in filenames
func sanitizeRepoName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Replace any character that's not alphanumeric or hyphen
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// ValidatePackages checks if packages are valid Pacman packages
func (g *Generator) ValidatePackages(packages []models.Package) error {
	for _, pkg := range packages {
		if pkg.Name == "" {
			return fmt.Errorf("package missing name: %s", pkg.Filename)
		}
		if pkg.Version == "" {
			return fmt.Errorf("package missing version: %s", pkg.Filename)
		}
		if pkg.Architecture == "" {
			return fmt.Errorf("package missing architecture: %s", pkg.Filename)
		}
		if !strings.Contains(pkg.Filename, ".pkg.tar.") {
			return fmt.Errorf("invalid package filename: %s", pkg.Filename)
		}
	}
	return nil
}

// GetSupportedType returns the package type supported by this generator
func (g *Generator) GetSupportedType() scanner.PackageType {
	return scanner.TypePacman
}
