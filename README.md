# Repogen - Universal Repository Generator

Repogen is a CLI tool that generates static repository structures for multiple package managers. It scans directories for packages, generates appropriate metadata files, and signs repositories with GPG/RSA keys.

## Supported Package Types

- **Debian/APT** (.deb packages)
- **Yum/RPM** (.rpm packages)
- **Alpine/APK** (.apk packages)
- **Homebrew** (bottle files)

## Features

- **Automatic Package Detection**: Scans directories and auto-detects package types using magic bytes
- **Metadata Generation**: Creates all necessary index and metadata files for each repository type
- **Repository Signing**: Signs repositories with GPG (Debian/RPM) or RSA (Alpine) keys
- **Unsigned Repository Support**:
  - Always generates InRelease files (required by Debian Trixie)
  - InRelease contains Release content without signature for unsigned repos
  - Compatible with `[trusted=yes]` apt option
- **Static Output**: Generates static file structures that can be served by any web server
- **Simple Component Structure**: Uses single component/pool structure for simplicity

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/ralt/repogen
cd repogen

# Build
go build -o repogen ./cmd/repogen

# Optional: Install to PATH
sudo cp repogen /usr/local/bin/
```

### Prerequisites

- Go 1.23 or later

## Usage

### Basic Usage

```bash
# Scan current directory and generate repositories
repogen generate

# Scan specific directory
repogen generate --input-dir /path/to/packages --output-dir /path/to/repo

# Enable verbose logging
repogen generate -v
```

### With Signing

#### Debian/RPM (GPG Signing)

```bash
# Generate signed Debian/RPM repositories
repogen generate \
  --input-dir ./packages \
  --output-dir ./repo \
  --gpg-key /path/to/private.key \
  --gpg-passphrase "your-passphrase"
```

#### Alpine (RSA Signing)

```bash
# Generate signed Alpine repository
repogen generate \
  --input-dir ./packages \
  --output-dir ./repo \
  --rsa-key /path/to/rsa-private.pem \
  --rsa-passphrase "your-passphrase" \
  --key-name "mykey"
```

### Configuration Options

```bash
repogen generate [flags]

Flags:
  # Input/Output
  -i, --input-dir string        Input directory to scan (default ".")
  -o, --output-dir string       Output directory (default "./repo")
  -v, --verbose                 Enable verbose logging

  # GPG Signing (Debian/RPM)
  -k, --gpg-key string          Path to GPG private key
  -p, --gpg-passphrase string   GPG key passphrase

  # RSA Signing (Alpine)
      --rsa-key string          Path to RSA private key
      --rsa-passphrase string   RSA key passphrase
      --key-name string         Key name for Alpine signatures (default "repogen")

  # Repository Metadata
      --origin string           Repository origin name
      --label string            Repository label
      --codename string         Codename for Debian repos (default "stable")
      --suite string            Suite for Debian repos (defaults to codename)
      --components strings      Components for Debian repos (default [main])
      --arch strings            Architectures to support (default [amd64])

  # Homebrew
      --base-url string         Base URL for Homebrew bottles
```

## Generated Repository Structures

### Debian/APT Repository

```
repo/
├── dists/
│   └── stable/
│       ├── InRelease              # Cleartext signed Release (or unsigned copy for unsigned repos)
│       ├── Release                # Main metadata
│       ├── Release.gpg            # Detached GPG signature (only for signed repos)
│       └── main/
│           └── binary-amd64/
│               ├── Packages        # Package metadata
│               ├── Packages.gz     # Compressed
│               └── Release
└── pool/
    └── main/
        └── {letter}/              # First letter of package name
            └── {package-name}/
                └── package.deb
```

**Using the Repository:**

```bash
# Add repository (unsigned)
echo "deb [trusted=yes] http://your-server.com/repo stable main" | sudo tee /etc/apt/sources.list.d/repo.list

# Add repository (signed)
# First, import the public key
wget -qO - http://your-server.com/repo/public.key | sudo apt-key add -
echo "deb http://your-server.com/repo stable main" | sudo tee /etc/apt/sources.list.d/repo.list

# Update and install
sudo apt update
sudo apt install package-name
```

### RPM/Yum Repository

```
repo/
├── repodata/
│   ├── repomd.xml              # Main metadata index
│   ├── repomd.xml.asc          # GPG signature
│   └── {hash}-primary.xml.gz   # Package metadata
└── Packages/
    └── *.rpm
```

**Using the Repository:**

```bash
# Create repo file
sudo tee /etc/yum.repos.d/repo.repo <<EOF
[myrepo]
name=My Repository
baseurl=http://your-server.com/repo
enabled=1
gpgcheck=0
EOF

# With GPG checking
sudo rpm --import http://your-server.com/repo/public.key
sudo tee /etc/yum.repos.d/repo.repo <<EOF
[myrepo]
name=My Repository
baseurl=http://your-server.com/repo
enabled=1
gpgcheck=1
gpgkey=http://your-server.com/repo/public.key
EOF

# Install packages
sudo yum install package-name
```

### Alpine/APK Repository

```
repo/
└── x86_64/
    ├── APKINDEX.tar.gz         # Package index
    ├── APKINDEX.tar.gz.SIGN.RSA.repogen.pub  # RSA signature
    └── package-1.0.0-r0.apk
```

**Using the Repository:**

```bash
# Add repository
echo "http://your-server.com/repo" | sudo tee -a /etc/apk/repositories

# With signing (copy public key first)
sudo cp repogen.pub /etc/apk/keys/
echo "http://your-server.com/repo" | sudo tee -a /etc/apk/repositories

# Update and install
sudo apk update
sudo apk add package-name
```

### Homebrew Tap

```
repo/
├── Formula/
│   └── package-name.rb         # Ruby formula
└── bottles/
    └── package--1.0.0.monterey.bottle.tar.gz
```

**Using the Repository:**

```bash
# Add tap (assuming repo is in GitHub)
brew tap username/repo https://github.com/username/repo

# Install package
brew install package-name
```

## GPG Key Setup

### Generate GPG Key for Signing

```bash
# Generate key
gpg --full-generate-key

# Export private key
gpg --export-secret-keys YOUR_KEY_ID > private.key

# Export public key (for distribution)
gpg --export --armor YOUR_KEY_ID > public.key
```

### Generate RSA Key for Alpine

```bash
# Generate RSA private key
openssl genrsa -out private.pem 2048

# Extract public key
openssl rsa -in private.pem -pubout -out public.pem

# With passphrase
openssl genrsa -aes256 -out private.pem 2048
```

## Repository Structure Details

### Debian Repository Format

Repogen generates Debian repositories following the standard format:
- **InRelease**: Cleartext signed Release file (preferred by modern apt). For unsigned repositories, contains the same content as Release file without signature wrapper.
- **Release**: Contains metadata and checksums of all index files
- **Release.gpg**: Detached signature of Release file (only for signed repositories)
- **Packages**: RFC 822-style package metadata
- **pool/**: Organized by first letter of package name

Key fields in Packages file:
- Package, Version, Architecture
- Filename (relative to repo root)
- Size, MD5sum, SHA1, SHA256, SHA512
- Description, Depends, Maintainer

### RPM Repository Format

Repogen generates RPM repositories compatible with yum/dnf:
- **repomd.xml**: Master index with checksums of metadata files
- **primary.xml.gz**: Core package information and dependencies
- Minimal metadata (primary only) for simplicity

The generated repositories can be consumed by:
- yum (RHEL/CentOS 7 and earlier)
- dnf (RHEL/CentOS 8+, Fedora)
- zypper (openSUSE)

### Alpine Repository Format

Repogen generates Alpine repositories in the apk v2 format:
- **APKINDEX.tar.gz**: Contains DESCRIPTION and APKINDEX files
- **APKINDEX**: Letter:value format package metadata
  - C: Checksum (Q1 prefix + base64 SHA1)
  - P: Package name
  - V: Version
  - A: Architecture
  - S: Size
  - T: Description
  - L: License
  - D: Dependencies (space-separated)

### Homebrew Tap Format

Repogen generates Homebrew taps with:
- **Formula/**: Ruby formula files auto-generated from bottles
- **bottles/**: Binary packages
- Multi-architecture support (arm64, x86_64)
- Platform detection from filename patterns

Bottle filename format: `{package}--{version}.{platform}.bottle.tar.gz`

## Examples

### Example 1: Simple Debian Repository

```bash
# Organize packages
mkdir -p packages
cp *.deb packages/

# Generate repository
repogen generate --input-dir packages --output-dir /var/www/repo

# Serve with nginx
sudo ln -s /var/www/repo /usr/share/nginx/html/repo
```

### Example 2: Multi-Architecture Debian Repository

```bash
repogen generate \
  --input-dir packages \
  --output-dir /var/www/repo \
  --arch amd64,arm64,i386 \
  --codename bookworm \
  --origin "My Company" \
  --label "Production Packages"
```

### Example 3: Signed RPM Repository

```bash
# Generate repository with GPG signing
repogen generate \
  --input-dir rpms \
  --output-dir /var/www/repo \
  --gpg-key ~/.gnupg/secring.gpg \
  --gpg-passphrase "secret"

# Export public key for users
gpg --export --armor YOUR_KEY_ID > /var/www/repo/RPM-GPG-KEY
```

### Example 4: Homebrew Tap with Multiple Bottles

```bash
# Organize bottles
mkdir bottles
cp *.bottle.tar.gz bottles/

# Generate tap
repogen generate \
  --input-dir bottles \
  --output-dir homebrew-tap \
  --base-url "https://github.com/username/homebrew-tap/releases/download/v1.0"
```

## Testing

Repogen includes a comprehensive test suite with Docker-based integration tests that verify each repository type works correctly in its native environment.

### Quick Start

```bash
# Build the binary
make build

# Build test packages
make test-packages

# Run all tests (unit + integration)
make test

# Run only integration tests
make test-integration
```

### Test Package Generation

Test packages are minimal dummy packages used to verify repository functionality:

```bash
# Build test packages natively (requires dpkg-deb, rpmbuild)
make test-packages

# Build test packages using Docker (recommended if tools not available)
make test-packages-docker
```

This creates:
- `test/fixtures/debs/repogen-test_1.0.0_amd64.deb`
- `test/fixtures/rpms/repogen-test-1.0.0-1.x86_64.rpm`
- `test/fixtures/apks/repogen-test-1.0.0-r0.apk`
- `test/fixtures/bottles/repogen-test--1.0.0.x86_64_linux.bottle.tar.gz`

### Integration Tests

Integration tests use Docker to:
1. Generate repositories with test packages
2. Spin up distribution-specific containers
3. Configure package managers to use test repositories
4. Install test packages
5. Verify successful installation

**Tested Distributions:**
- **Debian**: Debian Bookworm and Trixie containers
- **RPM**: Fedora latest container
- **Alpine**: Alpine latest container
- **Homebrew**: Formula validation (local)

**Running Integration Tests:**

```bash
# Requires Docker
make test-integration

# Or run directly with Go
go test -v -timeout 15m ./test

# Skip integration tests if Docker not available
go test -v -short ./...
```

### Test Output

Integration tests verify:
- ✓ Repository structure (all expected files present)
- ✓ Metadata files (Release, Packages, APKINDEX, repomd.xml)
- ✓ Package manager can read repository metadata
- ✓ Package manager can install packages
- ✓ Installed binaries execute successfully

Example output:
```
=== RUN   TestIntegration
=== RUN   TestIntegration/Debian
    Generating Debian repository...
    Testing repository in Debian container...
    ✓ Debian repository test passed
=== RUN   TestIntegration/RPM
    Generating RPM repository...
    Testing repository in Fedora container...
    ✓ RPM repository test passed
=== RUN   TestIntegration/Alpine
    Generating Alpine repository...
    Testing repository in Alpine container...
    ✓ Alpine repository test passed
=== RUN   TestIntegration/Homebrew
    Generating Homebrew repository...
    ✓ Homebrew repository test passed
```

### Continuous Integration

Example GitHub Actions workflow:

```yaml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Build test packages
        run: make test-packages-docker

      - name: Run tests
        run: make test
```

### Manual Testing

You can manually test repositories:

```bash
# Generate a test repository
./repogen generate --input-dir test/fixtures/debs --output-dir /tmp/test-repo

# Serve with Python
cd /tmp/test-repo
python3 -m http.server 8000

# In another terminal, test with Docker
docker run -it --rm debian:bookworm bash
# Inside container:
echo "deb [trusted=yes] http://host.docker.internal:8000 stable main" > /etc/apt/sources.list.d/test.list
apt update
apt install repogen-test
```

## Troubleshooting

### No packages found

- Check that package files have correct extensions (.deb, .rpm, .apk, .bottle.tar.gz)
- Verify magic bytes in files (packages may be corrupted)
- Use `--verbose` flag to see detailed scanning output

### GPG signing fails

- Verify GPG key is not encrypted or provide correct passphrase
- Check that private key file is readable
- Ensure go-crypto library supports your key type

### Repository not working

- Verify all metadata files were generated in output directory
- Check file permissions (should be readable by web server)
- Test with unsigned repository first (`[trusted=yes]` for apt)
- Review web server logs for 404s

### Debian Trixie requires InRelease files

Even with `[trusted=yes]`, Debian Trixie expects InRelease files to exist. Repogen now automatically generates InRelease files for all repositories:
- **Signed repositories**: InRelease contains cleartext signature
- **Unsigned repositories**: InRelease contains Release content (no signature)

This ensures compatibility with both old (Bookworm) and new (Trixie) Debian releases.

### Integration tests fail

- Ensure Docker is installed and running: `docker version`
- Build test packages first: `make test-packages`
- Check Docker can pull images: `docker pull debian:bookworm`
- Increase timeout for slow systems: `go test -timeout 30m ./test`

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for bugs and feature requests.

### Development Workflow

```bash
# Clone and build
git clone https://github.com/ralt/repogen
cd repogen
make build

# Make changes and test
make fmt              # Format code
make lint             # Run linter (requires golangci-lint)
make test             # Run all tests

# Before committing
make test-packages    # Ensure test packages build
make test             # Ensure all tests pass
```

## License

MIT License.

## For Maintainers

### Creating a Release

See **[RELEASING.md](RELEASING.md)** for detailed instructions on:
- Preparing and creating releases
- What happens during the automated release workflow
- Deploying the generated repository archive to S3 or web servers
- Troubleshooting common issues

Quick start:
```bash
# Run tests
make test

# Create and push a tag
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0

# GitHub Actions will automatically:
# - Build binaries for 4 platforms
# - Create native packages (deb, rpm, apk, bottle)
# - Generate a repository using repogen itself
# - Create release with all artifacts + repository archive
```

### CI/CD Workflows

- **Test Workflow** (`.github/workflows/test.yml`): Runs on PRs and pushes to main
- **Release Workflow** (`.github/workflows/release.yml`): Runs on version tags (v*.*.*)

The release workflow generates a `repogen-repository-VERSION.zip` archive containing a complete repository that you can extract and deploy to S3, GitHub Pages, or any web server.

## Acknowledgments

- Built with [spf13/cobra](https://github.com/spf13/cobra) for CLI
- Uses [ProtonMail/go-crypto](https://github.com/ProtonMail/go-crypto) for GPG operations
- Uses [sassoftware/go-rpmutils](https://github.com/sassoftware/go-rpmutils) for RPM parsing
- Uses [klauspost/compress](https://github.com/klauspost/compress) for fast compression

## See Also

- [Debian Repository Format](https://wiki.debian.org/DebianRepository/Format)
- [RPM Repository Format](https://rpm-software-management.github.io/)
- [Alpine APK Spec](https://wiki.alpinelinux.org/wiki/Apk_spec)
- [Homebrew Tap Documentation](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
