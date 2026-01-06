package deb

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralt/repogen/internal/models"
)

func TestGenerateReleaseUnsigned(t *testing.T) {
	// Setup temp directory
	tmpDir, err := os.MkdirTemp("", "repogen-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create generator without signer (unsigned)
	gen := NewGenerator(nil)

	config := &models.RepositoryConfig{
		OutputDir:  tmpDir,
		Codename:   "testing",
		Suite:      "testing",
		Origin:     "Test",
		Label:      "Test",
		Components: []string{"main"},
		Arches:     []string{"amd64"},
	}

	// Create required directory structure
	distsDir := filepath.Join(tmpDir, "dists", "testing", "main", "binary-amd64")
	os.MkdirAll(distsDir, 0755)

	// Create dummy Packages file
	packagesPath := filepath.Join(distsDir, "Packages")
	os.WriteFile(packagesPath, []byte("Package: test\n"), 0644)

	packagesGzPath := filepath.Join(distsDir, "Packages.gz")
	os.WriteFile(packagesGzPath, []byte{}, 0644)

	// Generate repository files
	err = gen.Generate(context.Background(), config, []models.Package{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify InRelease exists
	inReleasePath := filepath.Join(tmpDir, "dists", "testing", "InRelease")
	if _, err := os.Stat(inReleasePath); os.IsNotExist(err) {
		t.Errorf("InRelease not created for unsigned repository")
	}

	// Verify Release exists
	releasePath := filepath.Join(tmpDir, "dists", "testing", "Release")
	if _, err := os.Stat(releasePath); os.IsNotExist(err) {
		t.Errorf("Release not created")
	}

	// Verify Release.gpg does NOT exist
	releaseGpgPath := filepath.Join(tmpDir, "dists", "testing", "Release.gpg")
	if _, err := os.Stat(releaseGpgPath); !os.IsNotExist(err) {
		t.Errorf("Release.gpg should not exist for unsigned repository")
	}

	// Verify InRelease and Release have identical content
	inReleaseData, _ := os.ReadFile(inReleasePath)
	releaseData, _ := os.ReadFile(releasePath)

	if !bytes.Equal(inReleaseData, releaseData) {
		t.Errorf("InRelease content doesn't match Release content for unsigned repo")
		t.Logf("InRelease:\n%s", inReleaseData)
		t.Logf("Release:\n%s", releaseData)
	}

	// Verify InRelease does NOT contain PGP signature markers
	if bytes.Contains(inReleaseData, []byte("BEGIN PGP")) {
		t.Errorf("Unsigned InRelease should not contain PGP signature markers")
	}
}

func TestIncrementalModeCopiesNewPackages(t *testing.T) {
	// This test simulates the S3 workflow where:
	// 1. Initial repo is created with package A
	// 2. Only metadata is synced locally (not package files)
	// 3. Incremental mode adds package B
	// 4. New package B should be copied to output
	// 5. Metadata should reference both A and B

	// Setup temp directories
	tmpDir, err := os.MkdirTemp("", "repogen-test-incremental-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	inputDir := filepath.Join(tmpDir, "input")
	outputDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(inputDir, 0755)
	os.MkdirAll(outputDir, 0755)

	gen := NewGenerator(nil)
	config := &models.RepositoryConfig{
		OutputDir:  outputDir,
		Codename:   "testing",
		Suite:      "testing",
		Origin:     "Test",
		Label:      "Test",
		Components: []string{"main"},
		Arches:     []string{"amd64"},
	}

	// Step 1: Create initial repo with package A
	initialPkg := filepath.Join(inputDir, "pkga_1.0_amd64.deb")
	os.WriteFile(initialPkg, []byte("fake deb package A"), 0644)

	packagesA := []models.Package{
		{
			Name:         "pkga",
			Version:      "1.0",
			Architecture: "amd64",
			Filename:     initialPkg,
			Size:         18,
			MD5Sum:       "abc123",
			SHA1Sum:      "def456",
			SHA256Sum:    "ghi789",
		},
	}

	// Generate initial repo
	err = gen.Generate(context.Background(), config, packagesA)
	if err != nil {
		t.Fatalf("Initial generation failed: %v", err)
	}

	// Verify package A was copied
	pkgAPath := filepath.Join(outputDir, "pool", "main", "p", "pkga", "pkga_1.0_amd64.deb")
	if _, err := os.Stat(pkgAPath); os.IsNotExist(err) {
		t.Fatalf("Package A was not copied to pool: %v", err)
	}

	// Step 2: Simulate S3 sync - keep only metadata, remove package files
	// Remove pool directory to simulate only having metadata
	poolDir := filepath.Join(outputDir, "pool")
	os.RemoveAll(poolDir)

	// Verify package A is gone (simulating S3 scenario)
	if _, err := os.Stat(pkgAPath); !os.IsNotExist(err) {
		t.Fatalf("Package A should not exist locally after simulated S3 sync")
	}

	// Step 3: Create new package B
	newPkg := filepath.Join(inputDir, "pkgb_1.0_amd64.deb")
	os.WriteFile(newPkg, []byte("fake deb package B"), 0644)

	// Step 4: Parse existing metadata (simulating incremental mode)
	existingPackages, err := gen.ParseExistingMetadata(config)
	if err != nil {
		t.Fatalf("Failed to parse existing metadata: %v", err)
	}

	if len(existingPackages) != 1 {
		t.Fatalf("Expected 1 existing package, got %d", len(existingPackages))
	}

	if existingPackages[0].Name != "pkga" {
		t.Errorf("Expected existing package to be pkga, got %s", existingPackages[0].Name)
	}

	// Step 5: Run incremental generation with package B
	packagesB := []models.Package{
		{
			Name:         "pkgb",
			Version:      "1.0",
			Architecture: "amd64",
			Filename:     newPkg,
			Size:         18,
			MD5Sum:       "xyz123",
			SHA1Sum:      "uvw456",
			SHA256Sum:    "rst789",
		},
	}

	// Combine existing + new packages (simulating incremental mode)
	allPackages := append(existingPackages, packagesB...)

	err = gen.Generate(context.Background(), config, allPackages)
	if err != nil {
		t.Fatalf("Incremental generation failed: %v", err)
	}

	// Step 6: Verify new package B was copied
	pkgBPath := filepath.Join(outputDir, "pool", "main", "p", "pkgb", "pkgb_1.0_amd64.deb")
	if _, err := os.Stat(pkgBPath); os.IsNotExist(err) {
		t.Errorf("NEW package B was NOT copied to pool in incremental mode")
	}

	// Verify package B content
	pkgBContent, err := os.ReadFile(pkgBPath)
	if err != nil {
		t.Errorf("Failed to read package B: %v", err)
	} else if string(pkgBContent) != "fake deb package B" {
		t.Errorf("Package B has wrong content: %s", pkgBContent)
	}

	// Step 7: Verify metadata includes both packages
	packagesFile := filepath.Join(outputDir, "dists", "testing", "main", "binary-amd64", "Packages")
	packagesContent, err := os.ReadFile(packagesFile)
	if err != nil {
		t.Fatalf("Failed to read Packages file: %v", err)
	}

	packagesStr := string(packagesContent)
	if !bytes.Contains(packagesContent, []byte("Package: pkga")) {
		t.Errorf("Packages file should include pkga (existing package)")
	}
	if !bytes.Contains(packagesContent, []byte("Package: pkgb")) {
		t.Errorf("Packages file should include pkgb (new package)")
	}

	t.Logf("Incremental mode test passed!")
	t.Logf("Packages file content:\n%s", packagesStr)
}
