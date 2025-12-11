package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegration runs Docker-based integration tests for all repository types
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping integration tests")
	}

	// Get project root
	projectRoot, err := getProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	// Build repogen binary
	t.Log("Building repogen binary...")
	if err := buildRepogen(projectRoot); err != nil {
		t.Fatalf("Failed to build repogen: %v", err)
	}

	// Setup test directory
	testDir := filepath.Join(projectRoot, "test", "integration-output")
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatalf("Failed to clean test directory: %v", err)
	}
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Run tests for each repository type
	t.Run("Debian", func(t *testing.T) {
		testDebianRepository(t, projectRoot, testDir)
	})

	t.Run("DebianSigned", func(t *testing.T) {
		testDebianSignedRepository(t, projectRoot, testDir)
	})

	t.Run("DebianSignedTrixie", func(t *testing.T) {
		testDebianSignedTrixieRepository(t, projectRoot, testDir)
	})

	t.Run("DebianTrixie", func(t *testing.T) {
		testDebianTrixieRepository(t, projectRoot, testDir)
	})

	t.Run("RPM", func(t *testing.T) {
		testRPMRepository(t, projectRoot, testDir)
	})

	t.Run("Alpine", func(t *testing.T) {
		testAlpineRepository(t, projectRoot, testDir)
	})

	t.Run("Homebrew", func(t *testing.T) {
		testHomebrewRepository(t, projectRoot, testDir)
	})

	t.Run("Pacman", func(t *testing.T) {
		testPacmanRepository(t, projectRoot, testDir)
	})
}

func testDebianRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "debian-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "debs")

	// Check if test packages exist
	if _, err := os.Stat(filepath.Join(fixturesDir, "repogen-test_1.0.0_amd64.deb")); os.IsNotExist(err) {
		t.Skip("Debian test packages not found, run build-test-packages.sh first")
	}

	// Generate repository
	t.Log("Generating Debian repository with 3 packages...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--codename", "testing",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify repository structure (3 packages)
	expectedFiles := []string{
		"dists/testing/Release",
		"dists/testing/main/binary-amd64/Packages",
		"dists/testing/main/binary-amd64/Packages.gz",
		"pool/main/r/repogen-test/repogen-test_1.0.0_amd64.deb",
		"pool/main/r/repogen-utils/repogen-utils_2.0.0_amd64.deb",
		"pool/main/r/repogen-gzipped/repogen-gzipped_3.0.0_amd64.deb",
	}
	for _, file := range expectedFiles {
		path := filepath.Join(repoDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Verify InRelease exists (should always be generated now)
	inReleasePath := filepath.Join(repoDir, "dists", "testing", "InRelease")
	if _, err := os.Stat(inReleasePath); os.IsNotExist(err) {
		t.Errorf("InRelease file not found: %s", inReleasePath)
	}

	// Test repository in Docker
	t.Log("Testing repository in Debian container...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run Debian container and test repository
	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"debian:bookworm",
		"bash", "-c", `
set -e
echo "deb [trusted=yes] file:///repo testing main" > /etc/apt/sources.list.d/test.list
apt-get update
apt-cache policy repogen-test repogen-utils repogen-gzipped
apt-get install -y repogen-test repogen-utils repogen-gzipped
repogen-test
repogen-utils
repogen-gzipped
`,
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker test failed: %v", err)
	}

	t.Log("✓ Debian repository test passed")
}

func testDebianSignedRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "debian-signed-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "debs")
	gpgFixturesDir := filepath.Join(projectRoot, "test", "fixtures", "gpg-keys")

	// Check if test packages exist
	if _, err := os.Stat(filepath.Join(fixturesDir, "repogen-test_1.0.0_amd64.deb")); os.IsNotExist(err) {
		t.Skip("Debian test packages not found, run build-test-packages.sh first")
	}

	// Use fixture keys
	keyPath := filepath.Join(gpgFixturesDir, "test-key.asc")
	pubKeyPath := filepath.Join(gpgFixturesDir, "test-key-pub.asc")

	// Check if test keys exist
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatalf("Test GPG keys not found at %s", keyPath)
	}

	// Generate signed repository
	t.Log("Generating signed Debian repository...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--codename", "testing",
		"--gpg-key", keyPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate signed repository: %v\nOutput: %s", err, output)
	}

	// Verify InRelease is signed (contains PGP signature)
	inReleasePath := filepath.Join(repoDir, "dists", "testing", "InRelease")
	inReleaseData, err := os.ReadFile(inReleasePath)
	if err != nil {
		t.Fatalf("Failed to read InRelease: %v", err)
	}

	inReleaseContent := string(inReleaseData)
	if !strings.Contains(inReleaseContent, "-----BEGIN PGP SIGNED MESSAGE-----") {
		t.Errorf("InRelease missing PGP signed message header")
	}
	if !strings.Contains(inReleaseContent, "-----BEGIN PGP SIGNATURE-----") {
		t.Errorf("InRelease missing PGP signature block")
	}
	if !strings.Contains(inReleaseContent, "Hash: SHA512") {
		t.Errorf("InRelease missing Hash header")
	}

	// Verify Release.gpg exists
	releaseGpgPath := filepath.Join(repoDir, "dists", "testing", "Release.gpg")
	if _, err := os.Stat(releaseGpgPath); os.IsNotExist(err) {
		t.Errorf("Release.gpg not found for signed repository")
	}

	// Test repository in Docker with GPG verification (no trusted=yes!)
	t.Log("Testing signed repository in Debian container WITH signature verification...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Read public key for Docker
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"debian:bookworm",
		"bash", "-c", fmt.Sprintf(`
set -e

# Install GPG
apt-get update -qq
apt-get install -y -qq gnupg > /dev/null

# Import GPG public key
cat > /tmp/repo-key.asc <<'PUBKEY_EOF'
%s
PUBKEY_EOF
gpg --dearmor < /tmp/repo-key.asc > /etc/apt/trusted.gpg.d/repogen-test.gpg

# Add repository WITHOUT trusted=yes (signature will be verified!)
echo "deb file:///repo testing main" > /etc/apt/sources.list.d/test.list

# This should succeed with proper signature validation
apt-get update

# Verify packages are available
apt-cache policy repogen-test repogen-utils repogen-gzipped

# Install packages
apt-get install -y repogen-test repogen-utils repogen-gzipped

# Verify installed
repogen-test
repogen-utils
repogen-gzipped
`, string(pubKeyData)),
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker signed repository test failed: %v", err)
	}

	t.Log("✓ Signed Debian repository test passed (signature verified by APT)")
}

func testDebianSignedTrixieRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "debian-signed-trixie-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "debs")
	gpgFixturesDir := filepath.Join(projectRoot, "test", "fixtures", "gpg-keys")

	// Check if test packages exist
	if _, err := os.Stat(filepath.Join(fixturesDir, "repogen-test_1.0.0_amd64.deb")); os.IsNotExist(err) {
		t.Skip("Debian test packages not found, run build-test-packages.sh first")
	}

	// Use fixture keys
	keyPath := filepath.Join(gpgFixturesDir, "test-key.asc")
	pubKeyPath := filepath.Join(gpgFixturesDir, "test-key-pub.asc")

	// Check if test keys exist
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatalf("Test GPG keys not found at %s", keyPath)
	}

	// Generate signed repository
	t.Log("Generating signed Debian repository for Trixie...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--codename", "testing",
		"--gpg-key", keyPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate signed repository: %v\nOutput: %s", err, output)
	}

	// Verify InRelease is signed (contains PGP signature)
	inReleasePath := filepath.Join(repoDir, "dists", "testing", "InRelease")
	inReleaseData, err := os.ReadFile(inReleasePath)
	if err != nil {
		t.Fatalf("Failed to read InRelease: %v", err)
	}

	inReleaseContent := string(inReleaseData)
	if !strings.Contains(inReleaseContent, "-----BEGIN PGP SIGNED MESSAGE-----") {
		t.Errorf("InRelease missing PGP signed message header")
	}
	if !strings.Contains(inReleaseContent, "-----BEGIN PGP SIGNATURE-----") {
		t.Errorf("InRelease missing PGP signature block")
	}
	if !strings.Contains(inReleaseContent, "Hash: SHA512") {
		t.Errorf("InRelease missing Hash header")
	}

	// Test repository in Debian Trixie with GPG verification (no trusted=yes!)
	t.Log("Testing signed repository in Debian Trixie container WITH signature verification...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Read public key for Docker
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("Failed to read public key: %v", err)
	}

	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"debian:trixie",
		"bash", "-c", fmt.Sprintf(`
set -e

# Install GPG
apt-get update -qq
apt-get install -y -qq gnupg > /dev/null

# Import GPG public key
cat > /tmp/repo-key.asc <<'PUBKEY_EOF'
%s
PUBKEY_EOF
gpg --dearmor < /tmp/repo-key.asc > /etc/apt/trusted.gpg.d/repogen-test.gpg

# Add repository WITHOUT trusted=yes (signature will be verified!)
echo "deb file:///repo testing main" > /etc/apt/sources.list.d/test.list

# This should succeed with proper signature validation
apt-get update

# Verify packages are available
apt-cache policy repogen-test repogen-utils repogen-gzipped

# Install packages
apt-get install -y repogen-test repogen-utils repogen-gzipped

# Verify installed
repogen-test
repogen-utils
repogen-gzipped
`, string(pubKeyData)),
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker Trixie signed repository test failed: %v", err)
	}

	t.Log("✓ Signed Debian Trixie repository test passed (signature verified by APT)")
}

func testDebianTrixieRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "debian-trixie-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "debs")

	// Check if test packages exist
	if _, err := os.Stat(filepath.Join(fixturesDir, "repogen-test_1.0.0_amd64.deb")); os.IsNotExist(err) {
		t.Skip("Debian test packages not found, run build-test-packages.sh first")
	}

	// Generate unsigned repository (no GPG flags)
	t.Log("Generating unsigned Debian repository for Trixie testing...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--codename", "testing",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify InRelease exists for unsigned repository
	inReleasePath := filepath.Join(repoDir, "dists", "testing", "InRelease")
	if _, err := os.Stat(inReleasePath); os.IsNotExist(err) {
		t.Errorf("InRelease file not found for unsigned repository: %s", inReleasePath)
	}

	// Verify Release exists (backward compatibility)
	releasePath := filepath.Join(repoDir, "dists", "testing", "Release")
	if _, err := os.Stat(releasePath); os.IsNotExist(err) {
		t.Errorf("Release file not found: %s", releasePath)
	}

	// Verify Release.gpg does NOT exist (unsigned repo)
	releaseGpgPath := filepath.Join(repoDir, "dists", "testing", "Release.gpg")
	if _, err := os.Stat(releaseGpgPath); !os.IsNotExist(err) {
		t.Errorf("Release.gpg should not exist for unsigned repository")
	}

	// Verify InRelease content matches Release content (unsigned)
	inReleaseData, err := os.ReadFile(inReleasePath)
	if err != nil {
		t.Fatalf("Failed to read InRelease: %v", err)
	}
	releaseData, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatalf("Failed to read Release: %v", err)
	}
	if !bytes.Equal(inReleaseData, releaseData) {
		t.Errorf("InRelease content should match Release content for unsigned repository")
	}

	// Test repository in Debian Trixie container
	t.Log("Testing repository in Debian Trixie container...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"debian:trixie",
		"bash", "-c", `
set -e
echo "deb [trusted=yes] file:///repo testing main" > /etc/apt/sources.list.d/test.list
apt-get update
apt-cache policy repogen-test repogen-utils repogen-gzipped
apt-get install -y repogen-test repogen-utils repogen-gzipped
repogen-test
repogen-utils
repogen-gzipped
`,
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker Trixie test failed: %v", err)
	}

	t.Log("✓ Debian Trixie repository test passed")
}

func testRPMRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "rpm-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "rpms")

	// Check if test packages exist
	rpms, _ := filepath.Glob(filepath.Join(fixturesDir, "*.rpm"))
	if len(rpms) < 2 {
		t.Skip("RPM test packages not found (need 2), run build-test-packages.sh first")
	}

	// Generate repository
	t.Log("Generating RPM repository with 2 packages...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--base-url", "http://example.com/repo/",
		"--distro", "fedora",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify repository structure (2 packages) - new version/arch structure
	expectedFiles := []string{
		"40/x86_64/repodata/repomd.xml",
		"40/x86_64/Packages/repogen-test-1.0.0-1.x86_64.rpm",
		"40/x86_64/Packages/repogen-utils-2.0.0-1.x86_64.rpm",
		"fedora.repo",
	}
	for _, file := range expectedFiles {
		path := filepath.Join(repoDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Verify .repo file contains $releasever/$basearch
	repoFileContent, err := os.ReadFile(filepath.Join(repoDir, "fedora.repo"))
	if err != nil {
		t.Fatalf("Failed to read .repo file: %v", err)
	}
	repoContent := string(repoFileContent)
	if !strings.Contains(repoContent, "$releasever/$basearch") {
		t.Errorf(".repo file missing $releasever/$basearch variables. Content:\n%s", repoContent)
	}
	if !strings.Contains(repoContent, "[") {
		t.Errorf(".repo file appears to be malformed. Content:\n%s", repoContent)
	}

	// Test repository in Docker
	t.Log("Testing repository in Fedora container...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"fedora:latest",
		"bash", "-c", `
set -e
cat > /etc/yum.repos.d/test.repo <<EOF
[test]
name=Test Repository
baseurl=file:///repo/40/x86_64
enabled=1
gpgcheck=0
EOF
dnf install -y repogen-test repogen-utils
repogen-test
repogen-utils
`,
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker test failed: %v", err)
	}

	t.Log("✓ RPM repository test passed")
}

func testAlpineRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "alpine-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "apks")

	// Check if test packages exist
	apks, _ := filepath.Glob(filepath.Join(fixturesDir, "*.apk"))
	if len(apks) < 2 {
		t.Skip("Alpine test packages not found (need 2), run build-test-packages.sh first")
	}

	// Generate repository
	t.Log("Generating Alpine repository with 2 packages...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--arch", "x86_64",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify repository structure (2 packages)
	expectedFiles := []string{
		"x86_64/APKINDEX.tar.gz",
		"x86_64/repogen-test-1.0.0-r0.apk",
		"x86_64/repogen-utils-2.0.0-r0.apk",
	}
	for _, file := range expectedFiles {
		path := filepath.Join(repoDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Test repository structure (actual package installation requires signing)
	t.Log("Verifying APKINDEX structure...")
	apkindexPath := filepath.Join(repoDir, "x86_64", "APKINDEX.tar.gz")
	if _, err := os.Stat(apkindexPath); err != nil {
		t.Fatalf("APKINDEX.tar.gz not found: %v", err)
	}

	// Extract and verify APKINDEX content
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	tarCmd := exec.CommandContext(ctx, "tar", "-xzf", apkindexPath, "-O", "APKINDEX")
	output, err := tarCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to extract APKINDEX: %v\nOutput: %s", err, output)
	}

	// Verify APKINDEX contains both packages
	content := string(output)
	if !strings.Contains(content, "P:repogen-test") {
		t.Fatalf("APKINDEX does not contain first package name")
	}
	if !strings.Contains(content, "V:1.0.0-r0") {
		t.Fatalf("APKINDEX does not contain first package version")
	}
	if !strings.Contains(content, "P:repogen-utils") {
		t.Fatalf("APKINDEX does not contain second package name")
	}
	if !strings.Contains(content, "V:2.0.0-r0") {
		t.Fatalf("APKINDEX does not contain second package version")
	}
	if !strings.Contains(content, "A:x86_64") {
		t.Fatalf("APKINDEX does not contain architecture")
	}

	t.Log("✓ Alpine repository test passed (structure verification)")
	t.Log("Note: Full package installation requires GPG/RSA signing")
}

func testHomebrewRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "homebrew-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "bottles")

	// Check if test bottles exist
	bottles, _ := filepath.Glob(filepath.Join(fixturesDir, "*.bottle.tar.gz"))
	if len(bottles) < 2 {
		t.Skip("Homebrew test bottles not found (need 2), run build-test-packages.sh first")
	}

	// Generate repository
	t.Log("Generating Homebrew repository with 2 packages...")
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--base-url", "file:///repo",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify repository structure (2 packages)
	expectedFiles := []string{
		"Formula/repogen-test.rb",
		"Formula/repogen-utils.rb",
		"bottles/repogen-test--1.0.0.x86_64_linux.bottle.tar.gz",
		"bottles/repogen-utils--2.0.0.x86_64_linux.bottle.tar.gz",
	}
	for _, file := range expectedFiles {
		path := filepath.Join(repoDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Verify both formula files are valid Ruby
	formulas := map[string][]string{
		"repogen-test": {"class RepogenTest < Formula", "desc", "version"},
		"repogen-utils": {"class RepogenUtils < Formula", "desc", "version"},
	}

	for name, requiredStrings := range formulas {
		formulaPath := filepath.Join(repoDir, "Formula", name+".rb")
		formulaData, err := os.ReadFile(formulaPath)
		if err != nil {
			t.Fatalf("Failed to read formula %s: %v", name, err)
		}

		formulaContent := string(formulaData)
		for _, required := range requiredStrings {
			if !contains(formulaContent, required) {
				t.Errorf("Formula %s missing required content: %s", name, required)
			}
		}
	}

	t.Log("✓ Homebrew repository test passed")
}

func testPacmanRepository(t *testing.T, projectRoot, testDir string) {
	repoDir := filepath.Join(testDir, "pacman-repo")
	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "pacman")

	// Check if test packages exist
	pkgs, _ := filepath.Glob(filepath.Join(fixturesDir, "*.pkg.tar.*"))
	if len(pkgs) == 0 {
		t.Skip("Pacman test packages not found, run build-test-packages.sh first")
	}

	// Generate repository
	t.Logf("Generating Pacman repository with %d packages...", len(pkgs))
	repoGenBin := filepath.Join(projectRoot, "repogen")
	cmd := exec.Command(repoGenBin, "generate",
		"--input-dir", fixturesDir,
		"--output-dir", repoDir,
		"--origin", "test-repo",
		"--arch", "x86_64",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to generate repository: %v\nOutput: %s", err, output)
	}

	// Verify repository structure
	expectedFiles := []string{
		"x86_64/test-repo.db.tar.zst",
	}
	for _, file := range expectedFiles {
		path := filepath.Join(repoDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Verify packages were copied
	copiedPkgs, _ := filepath.Glob(filepath.Join(repoDir, "x86_64", "*.pkg.tar.*"))
	if len(copiedPkgs) == 0 {
		t.Error("No packages found in generated repository")
	}

	// Verify database structure
	t.Log("Verifying database structure...")
	dbPath := filepath.Join(repoDir, "x86_64", "test-repo.db.tar.zst")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// List database contents
	zstdCmd := exec.CommandContext(ctx, "zstd", "-d", "-c", dbPath)
	tarCmd := exec.CommandContext(ctx, "tar", "-t")

	zstdOut, err := zstdCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	tarCmd.Stdin = zstdOut

	output, err := tarCmd.CombinedOutput()
	if zstdCmd.Start() != nil || tarCmd.Start() != nil {
		t.Fatalf("Failed to start commands")
	}

	zstdCmd.Wait()
	if err := tarCmd.Wait(); err != nil {
		t.Fatalf("Failed to list database contents: %v\nOutput: %s", err, output)
	}

	content := string(output)
	t.Logf("Database contents:\n%s", content)

	// Verify database contains desc files
	if !strings.Contains(content, "/desc") {
		t.Error("Database missing desc files")
	}

	// Test repository in Docker (Arch Linux container)
	t.Log("Testing repository in Arch Linux container...")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dockerCmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/repo:ro", repoDir),
		"archlinux:latest",
		"bash", "-c", `
set -e

# Update package databases
pacman -Sy --noconfirm

# Add local repository (trusted, no signature verification)
cat >> /etc/pacman.conf <<EOF

[test-repo]
SigLevel = Optional TrustAll
Server = file:///repo/\$arch
EOF

# Update databases with new repo
pacman -Sy

# List available packages from test repo
echo "Available packages in test-repo:"
pacman -Sl test-repo

# Install package from test repo
echo "Installing nano from test-repo..."
pacman -S --noconfirm test-repo/nano

# Verify installation
echo "Verifying nano installation..."
nano --version
which nano

echo "✓ Package installed and verified successfully!"
`,
	)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		t.Fatalf("Docker test failed: %v", err)
	}

	t.Log("✓ Pacman repository test passed")
}

// Helper functions

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

func getProjectRoot() (string, error) {
	// Try to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find project root (go.mod)")
}

func buildRepogen(projectRoot string) error {
	cmd := exec.Command("go", "build", "-o", "repogen", "./cmd/repogen")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		len(s) > len(substr)+1 && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// generateTestGPGKey creates a test GPG key pair for repository signing tests
func generateTestGPGKey(privateKeyPath, publicKeyPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create GPG batch file for unattended key generation
	batchContent := `
%no-protection
Key-Type: RSA
Key-Length: 2048
Name-Real: Repogen Test Key
Name-Email: test@repogen.local
Expire-Date: 0
%commit
`

	tmpDir := filepath.Dir(privateKeyPath)
	batchFile := filepath.Join(tmpDir, "gpg-batch.txt")
	if err := os.WriteFile(batchFile, []byte(batchContent), 0600); err != nil {
		return fmt.Errorf("failed to create batch file: %w", err)
	}
	defer os.Remove(batchFile)

	// Generate key using gpg with temporary home directory
	gpgHome := filepath.Join(tmpDir, "gpg-home")
	if err := os.MkdirAll(gpgHome, 0700); err != nil {
		return fmt.Errorf("failed to create GPG home: %w", err)
	}

	// Generate the key
	cmd := exec.CommandContext(ctx, "gpg", "--homedir", gpgHome, "--batch", "--gen-key", batchFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate GPG key: %w\nOutput: %s", err, output)
	}

	// Export private key
	cmd = exec.CommandContext(ctx, "gpg", "--homedir", gpgHome, "--armor", "--export-secret-keys", "test@repogen.local")
	privateKey, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to export private key: %w", err)
	}
	if err := os.WriteFile(privateKeyPath, privateKey, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Export public key
	cmd = exec.CommandContext(ctx, "gpg", "--homedir", gpgHome, "--armor", "--export", "test@repogen.local")
	publicKey, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to export public key: %w", err)
	}
	if err := os.WriteFile(publicKeyPath, publicKey, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}
