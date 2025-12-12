package apk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/signer"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
)

// Generator implements the generator.Generator interface for Alpine repositories
type Generator struct {
	rsaSigner signer.RSASigner
	keyName   string
}

// NewGenerator creates a new Alpine generator
func NewGenerator(rsaSigner signer.RSASigner, keyName string) generator.Generator {
	return &Generator{
		rsaSigner: rsaSigner,
		keyName:   keyName,
	}
}

// Generate creates an Alpine repository structure
func (g *Generator) Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error {
	logrus.Info("Generating Alpine repository...")

	// Group packages by architecture
	archPackages := make(map[string][]models.Package)
	for _, pkg := range packages {
		arch := pkg.Architecture
		if arch == "" {
			arch = "x86_64"
		}
		archPackages[arch] = append(archPackages[arch], pkg)
	}

	// Generate repository for each architecture
	for _, arch := range config.Arches {
		if pkgs, ok := archPackages[arch]; ok {
			if err := g.generateForArch(ctx, config, arch, pkgs); err != nil {
				return fmt.Errorf("failed to generate for %s: %w", arch, err)
			}
		}
	}

	logrus.Info("Alpine repository generated successfully")
	return nil
}

// generateForArch generates repository files for a specific architecture
func (g *Generator) generateForArch(ctx context.Context, config *models.RepositoryConfig, arch string, packages []models.Package) error {
	logrus.Infof("Generating for architecture: %s", arch)

	// Create architecture directory
	archDir := filepath.Join(config.OutputDir, arch)
	if err := utils.EnsureDir(archDir); err != nil {
		return err
	}

	// Copy APK files to architecture directory and recalculate checksums
	for i := range packages {
		pkg := &packages[i]
		dstPath := filepath.Join(archDir, filepath.Base(pkg.Filename))
		if err := utils.CopyFile(pkg.Filename, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", pkg.Filename, err)
		}

		// Recalculate checksums on the copied file to ensure accuracy
		checksums, err := utils.CalculateChecksums(dstPath)
		if err != nil {
			return fmt.Errorf("failed to calculate checksums for %s: %w", filepath.Base(pkg.Filename), err)
		}
		pkg.Size = checksums.Size
		pkg.MD5Sum = checksums.MD5
		pkg.SHA1Sum = checksums.SHA1
		pkg.SHA256Sum = checksums.SHA256

		pkg.Filename = filepath.Base(pkg.Filename)
	}

	// Generate APKINDEX
	apkindexData, err := generateAPKINDEX(packages)
	if err != nil {
		return fmt.Errorf("failed to generate APKINDEX: %w", err)
	}

	// Create DESCRIPTION file
	descData := []byte(fmt.Sprintf("Alpine Package Index for %s", arch))

	// Package into tar.gz
	apkindexTarGz, err := createAPKINDEXTarGz(descData, apkindexData)
	if err != nil {
		return fmt.Errorf("failed to create APKINDEX.tar.gz: %w", err)
	}

	apkindexPath := filepath.Join(archDir, "APKINDEX.tar.gz")
	if err := utils.WriteFile(apkindexPath, apkindexTarGz, 0644); err != nil {
		return fmt.Errorf("failed to write APKINDEX.tar.gz: %w", err)
	}

	// Sign if signer available
	if g.rsaSigner != nil {
		signature, err := g.rsaSigner.SignRSA(apkindexTarGz)
		if err != nil {
			return fmt.Errorf("failed to sign APKINDEX: %w", err)
		}

		sigPath := filepath.Join(archDir, fmt.Sprintf("APKINDEX.tar.gz.SIGN.RSA.%s.pub", g.keyName))
		if err := utils.WriteFile(sigPath, signature, 0644); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}

		logrus.Info("APKINDEX signed successfully")
	}

	logrus.Infof("Generated APKINDEX for %s (%d packages)", arch, len(packages))
	return nil
}

// generateAPKINDEX creates an APKINDEX in Alpine's letter:value format
func generateAPKINDEX(packages []models.Package) ([]byte, error) {
	var buf bytes.Buffer

	for i, pkg := range packages {
		// Convert SHA1 hex string to bytes, then base64 encode with Q1 prefix
		sha1Bytes, err := hex.DecodeString(pkg.SHA1Sum)
		if err != nil {
			return nil, fmt.Errorf("failed to decode SHA1 for %s: %w", pkg.Name, err)
		}
		checksum := "Q1" + base64.StdEncoding.EncodeToString(sha1Bytes)

		fmt.Fprintf(&buf, "C:%s\n", checksum)
		fmt.Fprintf(&buf, "P:%s\n", pkg.Name)
		fmt.Fprintf(&buf, "V:%s\n", pkg.Version)
		fmt.Fprintf(&buf, "A:%s\n", pkg.Architecture)
		fmt.Fprintf(&buf, "S:%d\n", pkg.Size)

		if installedSize, ok := pkg.Metadata["installed_size"].(int64); ok {
			fmt.Fprintf(&buf, "I:%d\n", installedSize)
		}

		if pkg.Description != "" {
			fmt.Fprintf(&buf, "T:%s\n", pkg.Description)
		}

		if pkg.Homepage != "" {
			fmt.Fprintf(&buf, "U:%s\n", pkg.Homepage)
		}

		if pkg.License != "" {
			fmt.Fprintf(&buf, "L:%s\n", pkg.License)
		}

		if len(pkg.Dependencies) > 0 {
			fmt.Fprintf(&buf, "D:%s\n", strings.Join(pkg.Dependencies, " "))
		}

		// Blank line between packages (except last)
		if i < len(packages)-1 {
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}

// createAPKINDEXTarGz creates a tar.gz archive containing DESCRIPTION and APKINDEX
func createAPKINDEXTarGz(description, apkindex []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add DESCRIPTION file
	if err := addTarFile(tw, "DESCRIPTION", description); err != nil {
		return nil, err
	}

	// Add APKINDEX file
	if err := addTarFile(tw, "APKINDEX", apkindex); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// addTarFile adds a file to a tar archive
func addTarFile(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
		return err
	}

	return nil
}

// ValidatePackages checks if packages are valid APK packages
func (g *Generator) ValidatePackages(packages []models.Package) error {
	for _, pkg := range packages {
		if pkg.Name == "" {
			return fmt.Errorf("package missing name: %s", pkg.Filename)
		}
	}
	return nil
}

// GetSupportedType returns the package type this generator supports
func (g *Generator) GetSupportedType() scanner.PackageType {
	return scanner.TypeApk
}
