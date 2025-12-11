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
