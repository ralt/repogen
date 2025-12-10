# Release Process

This document describes how to create and publish a new release of Repogen.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Preparing a Release](#preparing-a-release)
- [Creating a Release](#creating-a-release)
- [What Happens During Release](#what-happens-during-release)
- [Post-Release Steps](#post-release-steps)
- [Deploying the Repository Archive](#deploying-the-repository-archive)
- [Troubleshooting](#troubleshooting)

## Prerequisites

Before creating a release, ensure you have:

- [ ] Write access to the repository
- [ ] All tests passing on main/master branch
- [ ] All planned features/fixes merged
- [ ] Updated CHANGELOG.md (if applicable)
- [ ] Git configured properly (`git config user.name` and `user.email`)

## Preparing a Release

### 1. Update Version References (Optional)

If your codebase has version strings, update them:

```bash
# Example: Update version in README badges, documentation, etc.
grep -r "v0.1.0" . --exclude-dir=.git
```

### 2. Run Tests Locally

Verify everything works before tagging:

```bash
# Run full test suite
make test

# Build test packages
make test-packages-docker

# Run integration tests
make test-integration

# Build the binary
make build
./repogen --help
```

### 3. Review Changes

Check what's changed since the last release:

```bash
# If you have a previous tag (e.g., v0.1.0)
git log v0.1.0..HEAD --oneline

# Or use GitHub compare
# https://github.com/yourusername/repogen/compare/v0.1.0...main
```

### 4. Decide on Version Number

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (v1.0.0 → v2.0.0): Breaking changes
- **MINOR** (v1.0.0 → v1.1.0): New features, backward compatible
- **PATCH** (v1.0.0 → v1.0.1): Bug fixes, backward compatible

## Creating a Release

### Step 1: Create and Push a Tag

```bash
# Make sure you're on the main branch and up to date
git checkout main
git pull origin main

# Create an annotated tag with a message
git tag -a v1.0.0 -m "Release version 1.0.0"

# Push the tag to GitHub
git push origin v1.0.0
```

**Important Notes:**
- Always use annotated tags (`-a` flag) with a message
- Tag format MUST be `v*.*.*` (e.g., v1.0.0, v0.2.1, v2.0.0-beta.1)
- The `v` prefix is required for the GitHub Actions workflow to trigger

### Step 2: Monitor the Release Workflow

After pushing the tag, GitHub Actions will automatically start the release workflow:

1. Go to: `https://github.com/yourusername/repogen/actions`
2. Look for the "Release" workflow run
3. Monitor the build progress (typically takes 5-10 minutes)

### Step 3: Verify the Release

Once the workflow completes:

1. Go to: `https://github.com/yourusername/repogen/releases`
2. Find your new release (should be marked "Latest")
3. Verify all artifacts are present:
   - ✅ Binaries (4): `repogen-linux-amd64`, `repogen-linux-arm64`, `repogen-darwin-amd64`, `repogen-darwin-arm64`
   - ✅ Debian package: `repogen_VERSION_amd64.deb`
   - ✅ RPM package: `repogen-VERSION-1.x86_64.rpm`
   - ✅ Alpine package: `repogen-VERSION-r0.apk`
   - ✅ Homebrew bottle: `repogen--VERSION.arm64_monterey.bottle.tar.gz`
   - ✅ Repository archives: `repogen-repository-VERSION.zip` and `.tar.gz`
   - ✅ Checksums: `SHA256SUMS`

## What Happens During Release

The `.github/workflows/release.yml` workflow performs these steps:

### 1. Build Phase

```
┌─────────────────────────────────────┐
│ Build Binaries (4 platforms)       │
│  • Linux amd64/arm64                │
│  • macOS amd64/arm64 (Intel/M1)    │
└─────────────────────────────────────┘
            ↓
┌─────────────────────────────────────┐
│ Create Native Packages              │
│  • Debian .deb (dpkg-deb)           │
│  • RPM .rpm (rpmbuild)              │
│  • Alpine .apk (tar.gz)             │
│  • Homebrew .bottle.tar.gz          │
└─────────────────────────────────────┘
```

### 2. Repository Generation Phase

```
┌─────────────────────────────────────┐
│ Generate Repository                 │
│  Uses repogen itself!               │
│  Input: dist/ (all packages)        │
│  Output: repo/ (full structure)     │
└─────────────────────────────────────┘
            ↓
┌─────────────────────────────────────┐
│ Repository Structure Created        │
│  repo/                              │
│  ├── dists/stable/                  │
│  │   └── Release, Packages          │
│  ├── pool/main/r/repogen/           │
│  │   └── *.deb                      │
│  ├── repodata/                      │
│  │   └── repomd.xml, *.xml.gz       │
│  ├── Packages/                      │
│  │   └── *.rpm                      │
│  ├── x86_64/                        │
│  │   └── APKINDEX.tar.gz, *.apk    │
│  └── Formula/                       │
│      └── repogen.rb                 │
└─────────────────────────────────────┘
```

### 3. Archive and Release Phase

```
┌─────────────────────────────────────┐
│ Create Archives                     │
│  • repogen-repository-VERSION.zip   │
│  • repogen-repository-VERSION.tar.gz│
└─────────────────────────────────────┘
            ↓
┌─────────────────────────────────────┐
│ Generate Checksums                  │
│  • SHA256SUMS (all files)           │
└─────────────────────────────────────┘
            ↓
┌─────────────────────────────────────┐
│ Create GitHub Release               │
│  • Upload all artifacts             │
│  • Add release notes                │
│  • Mark as latest                   │
└─────────────────────────────────────┘
```

## Post-Release Steps

### 1. Announce the Release

- Update project README with new version badges
- Post announcement on relevant channels
- Tweet/blog about new features (if applicable)

### 2. Update Documentation

If you maintain external documentation:

```bash
# Update download links
sed -i 's/v1.0.0/v1.1.0/g' docs/installation.md

# Commit and push
git add docs/
git commit -m "docs: Update to v1.1.0"
git push
```

### 3. Monitor for Issues

- Watch GitHub issues for bug reports
- Monitor CI/CD for any problems
- Check download statistics in GitHub Insights

## Deploying the Repository Archive

The repository archive (`.zip` or `.tar.gz`) is ready to deploy to any web server or S3 bucket.

### Option 1: Deploy to AWS S3

```bash
# Download the repository archive from GitHub releases
VERSION="1.0.0"
wget https://github.com/yourusername/repogen/releases/download/v${VERSION}/repogen-repository-${VERSION}.tar.gz

# Extract
tar xzf repogen-repository-${VERSION}.tar.gz

# Upload to S3
aws s3 sync . s3://your-bucket/packages/ \
  --acl public-read \
  --cache-control "max-age=3600"

# Optional: Set up CloudFront CDN
aws cloudfront create-invalidation \
  --distribution-id YOUR_DIST_ID \
  --paths "/*"
```

### Option 2: Deploy to GitHub Pages

```bash
# Clone your repository
git clone https://github.com/yourusername/homebrew-tap.git
cd homebrew-tap

# Download and extract repository
VERSION="1.0.0"
wget https://github.com/yourusername/repogen/releases/download/v${VERSION}/repogen-repository-${VERSION}.tar.gz
tar xzf repogen-repository-${VERSION}.tar.gz

# Commit and push
git add .
git commit -m "Release v${VERSION}"
git push origin main

# Enable GitHub Pages in repository settings
# Your repo will be available at: https://yourusername.github.io/homebrew-tap/
```

### Option 3: Deploy to Your Own Server

```bash
# SSH to your server
ssh user@yourserver.com

# Download and extract
cd /var/www/packages
VERSION="1.0.0"
wget https://github.com/yourusername/repogen/releases/download/v${VERSION}/repogen-repository-${VERSION}.tar.gz
tar xzf repogen-repository-${VERSION}.tar.gz

# Set permissions
chown -R www-data:www-data .
chmod -R 755 .

# Configure nginx or apache to serve the directory
# Users can now access: https://yourserver.com/packages/
```

### Option 4: Deploy to Netlify/Vercel

```bash
# Download and extract
VERSION="1.0.0"
wget https://github.com/yourusername/repogen/releases/download/v${VERSION}/repogen-repository-${VERSION}.zip
unzip repogen-repository-${VERSION}.zip -d repo/

# Deploy with Netlify CLI
cd repo
netlify deploy --prod

# Or drag and drop the 'repo' folder in Netlify web UI
```

## Users Can Now Install

After deploying, users can add your repository:

### Debian/Ubuntu
```bash
# Add repository
echo "deb [trusted=yes] https://yourserver.com/packages/ stable main" | sudo tee /etc/apt/sources.list.d/repogen.list

# Install
sudo apt update
sudo apt install repogen
```

### Fedora/RHEL
```bash
# Add repository
sudo tee /etc/yum.repos.d/repogen.repo <<EOF
[repogen]
name=Repogen Repository
baseurl=https://yourserver.com/packages/
enabled=1
gpgcheck=0
EOF

# Install
sudo dnf install repogen
```

### Alpine
```bash
# Add repository
echo "https://yourserver.com/packages/" | sudo tee -a /etc/apk/repositories

# Install
sudo apk add --allow-untrusted repogen
```

### Homebrew
```bash
# Add tap
brew tap yourusername/tap https://yourserver.com/packages/

# Install
brew install repogen
```

## Troubleshooting

### Release Workflow Failed

**Problem**: GitHub Actions workflow fails during release.

**Solutions**:
1. Check the Actions log for specific errors
2. Verify the tag format is correct (`v*.*.*`)
3. Ensure all required permissions are set in workflow file
4. Check if Docker is available (for package building)

### Tag Already Exists

**Problem**: You need to recreate a tag.

**Solution**:
```bash
# Delete local tag
git tag -d v1.0.0

# Delete remote tag
git push origin :refs/tags/v1.0.0

# Recreate and push
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

**Warning**: Only do this if the release hasn't been published yet!

### Missing Artifacts in Release

**Problem**: Some build artifacts are missing from the release.

**Solutions**:
1. Check if the build step failed in Actions log
2. Verify file paths in `release.yml`
3. Ensure the workflow has write permissions to releases
4. Manually re-run the workflow from Actions tab

### Repository Archive Structure Wrong

**Problem**: Generated repository has incorrect structure.

**Solutions**:
1. Test locally: `./repogen generate --input-dir dist --output-dir test-repo`
2. Verify package detection is working
3. Check repogen logs in the workflow
4. Ensure all package types are being built correctly

## Quick Reference

### Create a Release (Complete Steps)

```bash
# 1. Prepare
git checkout main
git pull origin main
make test

# 2. Create tag
git tag -a v1.0.0 -m "Release version 1.0.0"

# 3. Push tag
git push origin v1.0.0

# 4. Monitor at: https://github.com/yourusername/repogen/actions

# 5. Verify at: https://github.com/yourusername/repogen/releases

# 6. Deploy repository archive to S3/server
wget https://github.com/yourusername/repogen/releases/download/v1.0.0/repogen-repository-1.0.0.tar.gz
tar xzf repogen-repository-1.0.0.tar.gz
aws s3 sync . s3://your-bucket/packages/ --acl public-read
```

### Version Numbering Examples

```
v1.0.0       → First stable release
v1.0.1       → Bug fix
v1.1.0       → New feature (backward compatible)
v2.0.0       → Breaking changes
v1.0.0-beta.1 → Pre-release
v1.0.0-rc.1   → Release candidate
```

## CI/CD Workflows

### Test Workflow (`.github/workflows/test.yml`)

**Triggers**: Pull requests and pushes to main/master

**Purpose**: Run tests on every code change to ensure quality

**What it does**:
- Runs unit tests with race detector
- Builds test packages in Docker
- Runs full integration test suite
- Uploads code coverage

### Release Workflow (`.github/workflows/release.yml`)

**Triggers**: Git tags matching `v*.*.*`

**Purpose**: Automate the entire release process

**What it does**:
- Builds binaries for 4 platforms
- Creates native packages (deb, rpm, apk, bottle)
- Generates repository using repogen itself
- Archives repository for deployment
- Creates GitHub release with all artifacts
- Adds installation instructions

## Best Practices

1. **Always use annotated tags**: `git tag -a` not `git tag`
2. **Test before tagging**: Run `make test` locally first
3. **Use semantic versioning**: MAJOR.MINOR.PATCH format
4. **Write meaningful tag messages**: Describe what's new
5. **Don't delete published releases**: Only delete if absolutely necessary
6. **Keep CHANGELOG.md updated**: Document changes between versions
7. **Test the artifacts**: Download and test at least one artifact type
8. **Deploy promptly**: Deploy repository archive soon after release

## Need Help?

- 📖 Read the [README.md](README.md) for general usage
- 🐛 Report issues on [GitHub Issues](https://github.com/yourusername/repogen/issues)
- 💬 Ask questions in [GitHub Discussions](https://github.com/yourusername/repogen/discussions)
- 📧 Contact maintainers (see MAINTAINERS.md or repository settings)
