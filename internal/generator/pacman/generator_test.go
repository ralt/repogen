package pacman

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ralt/repogen/internal/models"
)

func TestGenerateDescFile(t *testing.T) {
	pkg := models.Package{
		Name:         "test-package",
		Version:      "1.0.0-1",
		Architecture: "x86_64",
		Description:  "Test package description",
		Maintainer:   "Test Maintainer <test@example.com>",
		Homepage:     "https://example.com",
		License:      "MIT",
		Filename:     "test-package-1.0.0-1-x86_64.pkg.tar.zst",
		Size:         12345,
		MD5Sum:       "abc123",
		SHA256Sum:    "def456",
		Dependencies: []string{"dep1", "dep2>=1.0"},
		Metadata: map[string]interface{}{
			"BuildDate":     "1234567890",
			"InstalledSize": "54321",
		},
	}

	desc, err := generateDescFile(pkg)
	if err != nil {
		t.Fatalf("Failed to generate desc file: %v", err)
	}

	descStr := string(desc)

	// Check required fields
	requiredFields := map[string]string{
		"%FILENAME%": "test-package-1.0.0-1-x86_64.pkg.tar.zst",
		"%NAME%":     "test-package",
		"%VERSION%":  "1.0.0-1",
		"%DESC%":     "Test package description",
		"%CSIZE%":    "12345",
		"%ISIZE%":    "54321",
		"%MD5SUM%":   "abc123",
		"%SHA256SUM%": "def456",
		"%ARCH%":      "x86_64",
		"%BUILDDATE%": "1234567890",
		"%PACKAGER%":  "Test Maintainer <test@example.com>",
		"%URL%":       "https://example.com",
		"%LICENSE%":   "MIT",
	}

	for field, value := range requiredFields {
		if !strings.Contains(descStr, field) {
			t.Errorf("desc file missing field: %s", field)
		}
		if !strings.Contains(descStr, value) {
			t.Errorf("desc file missing value for %s: %s", field, value)
		}
	}

	// Check dependencies
	if !strings.Contains(descStr, "%DEPENDS%") {
		t.Error("desc file missing %DEPENDS% section")
	}
	if !strings.Contains(descStr, "dep1") {
		t.Error("desc file missing dependency: dep1")
	}
	if !strings.Contains(descStr, "dep2>=1.0") {
		t.Error("desc file missing dependency: dep2>=1.0")
	}
}

func TestGenerateDatabase(t *testing.T) {
	config := &models.RepositoryConfig{
		Origin: "test-repo",
	}

	packages := []models.Package{
		{
			Name:         "pkg1",
			Version:      "1.0-1",
			Architecture: "x86_64",
			Description:  "Package 1",
			Filename:     "pkg1-1.0-1-x86_64.pkg.tar.zst",
			Size:         1000,
			MD5Sum:       "md5hash1",
			SHA256Sum:    "sha256hash1",
		},
		{
			Name:         "pkg2",
			Version:      "2.0-1",
			Architecture: "x86_64",
			Description:  "Package 2",
			Filename:     "pkg2-2.0-1-x86_64.pkg.tar.zst",
			Size:         2000,
			MD5Sum:       "md5hash2",
			SHA256Sum:    "sha256hash2",
		},
	}

	gen := &Generator{}
	dbData, err := gen.generateDatabase(config, packages)
	if err != nil {
		t.Fatalf("Failed to generate database: %v", err)
	}

	// Verify database is not empty
	if len(dbData) == 0 {
		t.Error("Generated database is empty")
	}

	// Decompress and verify structure
	zr, err := zstd.NewReader(bytes.NewReader(dbData))
	if err != nil {
		t.Fatalf("Failed to create zstd reader: %v", err)
	}
	defer zr.Close()

	decompressed := new(bytes.Buffer)
	if _, err := decompressed.ReadFrom(zr); err != nil {
		t.Fatalf("Failed to decompress database: %v", err)
	}

	// Check that database contains expected package directories
	content := decompressed.String()
	if !strings.Contains(content, "pkg1-1.0-1") {
		t.Error("Database missing pkg1-1.0-1 directory")
	}
	if !strings.Contains(content, "pkg2-2.0-1") {
		t.Error("Database missing pkg2-2.0-1 directory")
	}
}

func TestGenerateUnsigned(t *testing.T) {
	// Setup temp directory
	tmpDir, err := os.MkdirTemp("", "repogen-pacman-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create generator without signer (unsigned)
	gen := NewGenerator(nil)

	config := &models.RepositoryConfig{
		OutputDir: tmpDir,
		Origin:    "test-repo",
		Arches:    []string{"x86_64"},
	}

	// Create a minimal test package structure (we won't actually copy files)
	packages := []models.Package{
		{
			Name:         "test-pkg",
			Version:      "1.0-1",
			Architecture: "x86_64",
			Description:  "Test package",
			Filename:     filepath.Join(tmpDir, "test-pkg-1.0-1-x86_64.pkg.tar.zst"),
			Size:         100,
			MD5Sum:       "test-md5",
			SHA256Sum:    "test-sha256",
		},
	}

	// Create dummy package file
	os.WriteFile(packages[0].Filename, []byte("dummy"), 0644)

	// Generate repository files
	err = gen.Generate(context.Background(), config, packages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify database exists in arch directory
	dbPath := filepath.Join(tmpDir, "x86_64", "test-repo.db.tar.zst")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database not created: %s", dbPath)
	}

	// Verify signature does NOT exist
	sigPath := filepath.Join(tmpDir, "x86_64", "test-repo.db.tar.zst.sig")
	if _, err := os.Stat(sigPath); !os.IsNotExist(err) {
		t.Errorf("Signature file should not exist for unsigned repository")
	}

	// Verify package file exists
	pkgPath := filepath.Join(tmpDir, "x86_64", "test-pkg-1.0-1-x86_64.pkg.tar.zst")
	if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
		t.Errorf("Package file not copied: %s", pkgPath)
	}
}

func TestGenerateCreatesDbFile(t *testing.T) {
	// Setup temp directory
	tmpDir, err := os.MkdirTemp("", "repogen-pacman-db-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create generator without signer
	gen := NewGenerator(nil)

	config := &models.RepositoryConfig{
		OutputDir: tmpDir,
		RepoName:  "test-repo",
		Arches:    []string{"x86_64"},
	}

	// Create a minimal test package
	packages := []models.Package{
		{
			Name:         "test-pkg",
			Version:      "1.0-1",
			Architecture: "x86_64",
			Description:  "Test package",
			Filename:     filepath.Join(tmpDir, "test-pkg-1.0-1-x86_64.pkg.tar.zst"),
			Size:         100,
			MD5Sum:       "test-md5",
			SHA256Sum:    "test-sha256",
		},
	}

	// Create dummy package file
	os.WriteFile(packages[0].Filename, []byte("dummy"), 0644)

	// Generate repository
	err = gen.Generate(context.Background(), config, packages)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify .db.tar.zst exists
	dbTarPath := filepath.Join(tmpDir, "x86_64", "test-repo.db.tar.zst")
	dbTarInfo, err := os.Stat(dbTarPath)
	if os.IsNotExist(err) {
		t.Errorf("Database file not created: %s", dbTarPath)
	}

	// Verify .db file exists
	dbPath := filepath.Join(tmpDir, "x86_64", "test-repo.db")
	dbInfo, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		t.Errorf("Database .db file not created: %s", dbPath)
	}

	// Verify .db is a copy (same size as .db.tar.zst)
	if dbInfo != nil && dbTarInfo != nil {
		if dbInfo.Size() != dbTarInfo.Size() {
			t.Errorf(".db file size (%d) doesn't match .db.tar.zst size (%d)", dbInfo.Size(), dbTarInfo.Size())
		}
		if dbInfo.Size() == 0 {
			t.Error(".db file is empty")
		}
	}
}

func TestValidatePackages(t *testing.T) {
	gen := &Generator{}

	validPkg := models.Package{
		Name:         "valid-pkg",
		Version:      "1.0-1",
		Architecture: "x86_64",
		Filename:     "valid-pkg-1.0-1-x86_64.pkg.tar.zst",
	}

	// Test valid package
	if err := gen.ValidatePackages([]models.Package{validPkg}); err != nil {
		t.Errorf("Valid package failed validation: %v", err)
	}

	// Test missing name
	invalidPkg := validPkg
	invalidPkg.Name = ""
	if err := gen.ValidatePackages([]models.Package{invalidPkg}); err == nil {
		t.Error("Package with missing name should fail validation")
	}

	// Test missing version
	invalidPkg = validPkg
	invalidPkg.Version = ""
	if err := gen.ValidatePackages([]models.Package{invalidPkg}); err == nil {
		t.Error("Package with missing version should fail validation")
	}

	// Test missing architecture
	invalidPkg = validPkg
	invalidPkg.Architecture = ""
	if err := gen.ValidatePackages([]models.Package{invalidPkg}); err == nil {
		t.Error("Package with missing architecture should fail validation")
	}

	// Test invalid filename
	invalidPkg = validPkg
	invalidPkg.Filename = "invalid.tar.gz"
	if err := gen.ValidatePackages([]models.Package{invalidPkg}); err == nil {
		t.Error("Package with invalid filename should fail validation")
	}
}
