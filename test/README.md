# Integration Test Suite

This directory contains the Docker-based integration test suite for repogen.

## Overview

The test suite validates that repogen generates working repositories for all supported package types by:
1. Building minimal test packages
2. Generating repositories using repogen
3. Running native package managers in Docker containers
4. Verifying packages can be installed and executed

## Directory Structure

```
test/
├── build-test-packages.sh    # Script to build test packages
├── integration_test.go        # Go integration tests
├── README.md                  # This file
└── fixtures/                  # Test package fixtures
    ├── debs/                  # Debian test packages
    ├── rpms/                  # RPM test packages
    ├── apks/                  # Alpine test packages
    └── bottles/               # Homebrew test bottles
```

## Running Tests

### Prerequisites

- Docker (for running containers)
- Go 1.23+ (for running tests)
- `dpkg-deb` (for building Debian packages)
- `rpmbuild` (for building RPM packages)

Or use Docker to build all packages:
```bash
make test-packages-docker
```

### Quick Start

```bash
# Build test packages
./build-test-packages.sh

# Run integration tests
go test -v -timeout 15m .

# Or use Makefile
make test-integration
```

### Individual Tests

```bash
# Run only Debian tests
go test -v -run TestIntegration/Debian .

# Run only RPM tests
go test -v -run TestIntegration/RPM .

# Run only Alpine tests
go test -v -run TestIntegration/Alpine .

# Run only Homebrew tests
go test -v -run TestIntegration/Homebrew .
```

## Test Packages

### Debian Package

**File**: `fixtures/debs/repogen-test_1.0.0_amd64.deb`

Minimal Debian package with:
- Control file with metadata
- Binary: `/usr/bin/repogen-test`
- Architecture: amd64

### RPM Package

**File**: `fixtures/rpms/repogen-test-1.0.0-1.x86_64.rpm`

Minimal RPM package with:
- Spec file with metadata
- Binary: `/usr/bin/repogen-test`
- Architecture: x86_64

### Alpine APK Package

**File**: `fixtures/apks/repogen-test-1.0.0-r0.apk`

Minimal APK package with:
- .PKGINFO metadata file
- Binary: `/usr/bin/repogen-test`
- Architecture: x86_64

### Homebrew Bottle

**File**: `fixtures/bottles/repogen-test--1.0.0.x86_64_linux.bottle.tar.gz`

Minimal Homebrew bottle with:
- Binary: `repogen-test/1.0.0/bin/repogen-test`
- Platform: x86_64_linux

## Test Flow

Each integration test follows this pattern:

### 1. Setup
- Clean previous test outputs
- Verify test package exists
- Create output directory

### 2. Generate Repository
```bash
repogen generate \
  --input-dir fixtures/{type} \
  --output-dir test-output/{type}-repo
```

### 3. Verify Structure
Check that all expected files exist:
- Metadata files (Release, Packages, APKINDEX, etc.)
- Package files in correct locations
- Signatures (if signing enabled)

### 4. Docker Test
Spin up distribution-specific container:

**Debian:**
```bash
docker run --rm -v $(pwd)/repo:/repo debian:bookworm bash -c "
  echo 'deb [trusted=yes] file:///repo stable main' > /etc/apt/sources.list.d/test.list
  apt update
  apt install -y repogen-test
  repogen-test
"
```

**Fedora:**
```bash
docker run --rm -v $(pwd)/repo:/repo fedora:latest bash -c "
  cat > /etc/yum.repos.d/test.repo <<EOF
  [test]
  baseurl=file:///repo
  gpgcheck=0
  EOF
  dnf install -y repogen-test
  repogen-test
"
```

**Alpine:**
```bash
docker run --rm -v $(pwd)/repo:/repo alpine:latest sh -c "
  echo 'file:///repo' >> /etc/apk/repositories
  apk update --allow-untrusted
  apk add --allow-untrusted repogen-test
  repogen-test
"
```

### 5. Verification
- Package manager successfully reads repository
- Package installs without errors
- Binary executes successfully

## Test Output

### Success

```
=== RUN   TestIntegration
=== RUN   TestIntegration/Debian
    integration_test.go:45: Generating Debian repository...
    integration_test.go:70: Testing repository in Debian container...
    integration_test.go:98: ✓ Debian repository test passed
=== RUN   TestIntegration/RPM
    integration_test.go:115: Generating RPM repository...
    integration_test.go:134: Testing repository in Fedora container...
    integration_test.go:158: ✓ RPM repository test passed
=== RUN   TestIntegration/Alpine
    integration_test.go:175: Generating Alpine repository...
    integration_test.go:195: Testing repository in Alpine container...
    integration_test.go:219: ✓ Alpine repository test passed
=== RUN   TestIntegration/Homebrew
    integration_test.go:236: Generating Homebrew repository...
    integration_test.go:262: ✓ Homebrew repository test passed
--- PASS: TestIntegration (45.23s)
    --- PASS: TestIntegration/Debian (12.45s)
    --- PASS: TestIntegration/RPM (14.32s)
    --- PASS: TestIntegration/Alpine (10.21s)
    --- PASS: TestIntegration/Homebrew (8.25s)
PASS
ok      github.com/ralt/repogen/test    45.234s
```

### Failure Example

```
=== RUN   TestIntegration/Debian
    integration_test.go:55: Expected file not found: dists/testing/Release
--- FAIL: TestIntegration/Debian (2.34s)
```

## Troubleshooting

### Docker not available
```
--- SKIP: TestIntegration (0.00s)
    integration_test.go:22: Docker not available, skipping integration tests
```
**Solution**: Install Docker or run tests on CI

### Test package not found
```
--- SKIP: TestIntegration/Debian (0.00s)
    integration_test.go:42: Debian test package not found, run build-test-packages.sh first
```
**Solution**: Run `./build-test-packages.sh` or `make test-packages`

### Container fails to pull
```
Error: failed to pull image "debian:bookworm": ...
```
**Solution**: Check internet connection or Docker Hub access

### Permission denied
```
Error: permission denied while trying to connect to Docker daemon
```
**Solution**: Add user to docker group: `sudo usermod -aG docker $USER`

### Timeout
```
panic: test timed out after 15m0s
```
**Solution**: Increase timeout: `go test -timeout 30m .`

## Adding New Tests

To add a test for a new package format:

1. **Create test package builder** in `build-test-packages.sh`:
```bash
build_newformat() {
    echo "Building NewFormat test package..."
    # Build logic here
}
```

2. **Add test function** in `integration_test.go`:
```go
func testNewFormatRepository(t *testing.T, projectRoot, testDir string) {
    // Setup
    repoDir := filepath.Join(testDir, "newformat-repo")

    // Generate repository
    // ...

    // Verify structure
    // ...

    // Test in Docker
    // ...
}
```

3. **Add to test suite**:
```go
t.Run("NewFormat", func(t *testing.T) {
    testNewFormatRepository(t, projectRoot, testDir)
})
```

## CI Integration

The test suite is designed for CI/CD:
- Fast feedback (parallel container tests)
- Clear pass/fail indicators
- Detailed error messages
- Automatic cleanup

See `.github/workflows/test.yml` for GitHub Actions example.

## Performance

Typical test execution times:
- Test package build: ~5 seconds
- Repository generation: ~1-2 seconds per type
- Docker container tests: ~10-15 seconds per container
- **Total**: ~1-2 minutes

## Maintenance

Test packages should be rebuilt when:
- Package format specifications change
- New metadata fields are required
- Testing new architectures
- Updating to new OS versions

Rebuild: `make test-packages`
