package apk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	tmpDir, err := os.MkdirTemp("", "repogen-test-apk-incremental-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	inputDir := filepath.Join(tmpDir, "input")
	outputDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(inputDir, 0755)
	os.MkdirAll(outputDir, 0755)

	gen := NewGenerator(nil, "")
	config := &models.RepositoryConfig{
		OutputDir: outputDir,
		Arches:    []string{"x86_64"},
	}

	// Step 1: Create initial repo with package A
	initialPkg := filepath.Join(inputDir, "pkga-1.0-r1.apk")
	os.WriteFile(initialPkg, []byte("fake apk package A"), 0644)

	packagesA := []models.Package{
		{
			Name:         "pkga",
			Version:      "1.0-r1",
			Architecture: "x86_64",
			Description:  "Package A",
			Filename:     initialPkg,
			Size:         18,
			MD5Sum:       "abc123",
			SHA256Sum:    "ghi789",
		},
	}

	// Generate initial repo
	err = gen.Generate(context.Background(), config, packagesA)
	if err != nil {
		t.Fatalf("Initial generation failed: %v", err)
	}

	// Verify package A was copied
	pkgAPath := filepath.Join(outputDir, "x86_64", "pkga-1.0-r1.apk")
	if _, err := os.Stat(pkgAPath); os.IsNotExist(err) {
		t.Fatalf("Package A was not copied: %v", err)
	}

	// Step 2: Simulate S3 sync - keep only APKINDEX, remove package files
	archDir := filepath.Join(outputDir, "x86_64")
	files, _ := os.ReadDir(archDir)
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".apk") {
			os.Remove(filepath.Join(archDir, file.Name()))
		}
	}

	// Verify package A is gone (simulating S3 scenario)
	if _, err := os.Stat(pkgAPath); !os.IsNotExist(err) {
		t.Fatalf("Package A should not exist locally after simulated S3 sync")
	}

	// Step 3: Create new package B
	newPkg := filepath.Join(inputDir, "pkgb-1.0-r1.apk")
	os.WriteFile(newPkg, []byte("fake apk package B"), 0644)

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
			Version:      "1.0-r1",
			Architecture: "x86_64",
			Description:  "Package B",
			Filename:     newPkg,
			Size:         18,
			MD5Sum:       "xyz123",
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
	pkgBPath := filepath.Join(outputDir, "x86_64", "pkgb-1.0-r1.apk")
	if _, err := os.Stat(pkgBPath); os.IsNotExist(err) {
		t.Errorf("NEW package B was NOT copied in incremental mode")
	}

	// Verify package B content
	pkgBContent, err := os.ReadFile(pkgBPath)
	if err != nil {
		t.Errorf("Failed to read package B: %v", err)
	} else if string(pkgBContent) != "fake apk package B" {
		t.Errorf("Package B has wrong content: %s", pkgBContent)
	}

	// Step 7: Verify APKINDEX was regenerated
	apkindexPath := filepath.Join(outputDir, "x86_64", "APKINDEX.tar.gz")
	if _, err := os.Stat(apkindexPath); os.IsNotExist(err) {
		t.Errorf("APKINDEX.tar.gz not created: %s", apkindexPath)
	}

	t.Logf("Incremental mode test passed for APK!")
}
