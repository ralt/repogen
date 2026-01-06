package rpm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralt/repogen/internal/models"
)

func TestIncrementalModeCopiesNewPackages(t *testing.T) {
	// This test simulates the S3 workflow where:
	// 1. Initial repo is created with package A
	// 2. Only metadata is synced locally (not package files)
	// 3. Incremental mode adds package B
	// 4. New package B should be copied to output
	// 5. Metadata should reference both A and B

	// Setup temp directories
	tmpDir, err := os.MkdirTemp("", "repogen-test-rpm-incremental-")
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
		OutputDir:      outputDir,
		Version:        "40",
		DistroVariant:  "fedora",
		Arches:         []string{"x86_64"},
	}

	// Step 1: Create initial repo with package A
	initialPkg := filepath.Join(inputDir, "pkga-1.0-1.x86_64.rpm")
	os.WriteFile(initialPkg, []byte("fake rpm package A"), 0644)

	packagesA := []models.Package{
		{
			Name:         "pkga",
			Version:      "1.0",
			Architecture: "x86_64",
			Description:  "Package A",
			Filename:     initialPkg,
			Size:         18,
			MD5Sum:       "abc123",
			SHA256Sum:    "ghi789",
			Metadata: map[string]interface{}{
				"Release": "1",
			},
		},
	}

	// Generate initial repo
	err = gen.Generate(context.Background(), config, packagesA)
	if err != nil {
		t.Fatalf("Initial generation failed: %v", err)
	}

	// Verify package A was copied
	pkgAPath := filepath.Join(outputDir, "40", "x86_64", "Packages", "pkga-1.0-1.x86_64.rpm")
	if _, err := os.Stat(pkgAPath); os.IsNotExist(err) {
		t.Fatalf("Package A was not copied: %v", err)
	}

	// Step 2: Simulate S3 sync - keep only repodata, remove package files
	packagesDir := filepath.Join(outputDir, "40", "x86_64", "Packages")
	os.RemoveAll(packagesDir)

	// Verify package A is gone (simulating S3 scenario)
	if _, err := os.Stat(pkgAPath); !os.IsNotExist(err) {
		t.Fatalf("Package A should not exist locally after simulated S3 sync")
	}

	// Step 3: Create new package B
	newPkg := filepath.Join(inputDir, "pkgb-1.0-1.x86_64.rpm")
	os.WriteFile(newPkg, []byte("fake rpm package B"), 0644)

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
			Architecture: "x86_64",
			Description:  "Package B",
			Filename:     newPkg,
			Size:         18,
			MD5Sum:       "xyz123",
			SHA256Sum:    "rst789",
			Metadata: map[string]interface{}{
				"Release": "1",
			},
		},
	}

	// Combine existing + new packages (simulating incremental mode)
	allPackages := append(existingPackages, packagesB...)

	err = gen.Generate(context.Background(), config, allPackages)
	if err != nil {
		t.Fatalf("Incremental generation failed: %v", err)
	}

	// Step 6: Verify new package B was copied
	pkgBPath := filepath.Join(outputDir, "40", "x86_64", "Packages", "pkgb-1.0-1.x86_64.rpm")
	if _, err := os.Stat(pkgBPath); os.IsNotExist(err) {
		t.Errorf("NEW package B was NOT copied in incremental mode")
	}

	// Verify package B content
	pkgBContent, err := os.ReadFile(pkgBPath)
	if err != nil {
		t.Errorf("Failed to read package B: %v", err)
	} else if string(pkgBContent) != "fake rpm package B" {
		t.Errorf("Package B has wrong content: %s", pkgBContent)
	}

	// Step 7: Verify repodata was regenerated
	repodataDir := filepath.Join(outputDir, "40", "x86_64", "repodata")
	if _, err := os.Stat(repodataDir); os.IsNotExist(err) {
		t.Errorf("Repodata not created: %s", repodataDir)
	}

	// Verify repomd.xml exists
	repomdPath := filepath.Join(repodataDir, "repomd.xml")
	if _, err := os.Stat(repomdPath); os.IsNotExist(err) {
		t.Errorf("repomd.xml not created: %s", repomdPath)
	}

	t.Logf("Incremental mode test passed for RPM!")
}
