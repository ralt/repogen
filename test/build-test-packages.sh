#!/bin/bash
set -e

# Build test packages for integration testing
# This script creates minimal dummy packages for each format

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES_DIR="${SCRIPT_DIR}/fixtures"

echo "Building test packages..."

# Only clean build directories, preserve final packages
for dir in deb-build rpm-build apk-build bottle-build; do
    if [ -d "${FIXTURES_DIR}/${dir}" ]; then
        if [ -w "${FIXTURES_DIR}/${dir}" ]; then
            rm -rf "${FIXTURES_DIR}/${dir}"
        else
            # Try with sudo if available, otherwise skip
            if command -v sudo &> /dev/null; then
                sudo rm -rf "${FIXTURES_DIR}/${dir}" 2>/dev/null || true
            fi
        fi
    fi
done

# ============================================================================
# Build Debian Package
# ============================================================================
build_deb() {
    echo "Building Debian test packages..."

    DEB_BUILD_DIR="${FIXTURES_DIR}/deb-build"
    rm -rf "${DEB_BUILD_DIR}"

    # Build first package: repogen-test 1.0.0
    mkdir -p "${DEB_BUILD_DIR}/testpkg1/DEBIAN"
    mkdir -p "${DEB_BUILD_DIR}/testpkg1/usr/bin"

    cat > "${DEB_BUILD_DIR}/testpkg1/DEBIAN/control" <<EOF
Package: repogen-test
Version: 1.0.0
Section: misc
Priority: optional
Architecture: amd64
Maintainer: Repogen <test@example.com>
Description: Test package for repogen
 This is a minimal test package used to verify
 that repogen-generated repositories work correctly.
EOF

    cat > "${DEB_BUILD_DIR}/testpkg1/usr/bin/repogen-test" <<'EOF'
#!/bin/sh
echo "Repogen test package v1.0.0 installed successfully!"
EOF
    chmod +x "${DEB_BUILD_DIR}/testpkg1/usr/bin/repogen-test"

    dpkg-deb --build "${DEB_BUILD_DIR}/testpkg1" "${FIXTURES_DIR}/debs/repogen-test_1.0.0_amd64.deb"

    # Build second package: repogen-utils 2.0.0
    mkdir -p "${DEB_BUILD_DIR}/testpkg2/DEBIAN"
    mkdir -p "${DEB_BUILD_DIR}/testpkg2/usr/bin"

    cat > "${DEB_BUILD_DIR}/testpkg2/DEBIAN/control" <<EOF
Package: repogen-utils
Version: 2.0.0
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Repogen <test@example.com>
Description: Utility package for repogen
 This is a second test package to verify
 multi-package repository generation.
EOF

    cat > "${DEB_BUILD_DIR}/testpkg2/usr/bin/repogen-utils" <<'EOF'
#!/bin/sh
echo "Repogen utils package v2.0.0 installed successfully!"
EOF
    chmod +x "${DEB_BUILD_DIR}/testpkg2/usr/bin/repogen-utils"

    dpkg-deb --build "${DEB_BUILD_DIR}/testpkg2" "${FIXTURES_DIR}/debs/repogen-utils_2.0.0_amd64.deb"

    echo "✓ Debian packages created (2)"
}

# ============================================================================
# Build RPM Package
# ============================================================================
build_rpm() {
    echo "Building RPM test packages..."

    RPM_BUILD_DIR="${FIXTURES_DIR}/rpm-build"
    rm -rf "${RPM_BUILD_DIR}"
    mkdir -p "${RPM_BUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

    # Create first spec file: repogen-test
    cat > "${RPM_BUILD_DIR}/SPECS/repogen-test.spec" <<'EOF'
Name:           repogen-test
Version:        1.0.0
Release:        1
Summary:        Test package for repogen
License:        MIT
BuildArch:      x86_64

%description
This is a minimal test package used to verify
that repogen-generated repositories work correctly.

%install
mkdir -p %{buildroot}/usr/bin
cat > %{buildroot}/usr/bin/repogen-test <<'SCRIPT'
#!/bin/sh
echo "Repogen test package v1.0.0 installed successfully!"
SCRIPT
chmod +x %{buildroot}/usr/bin/repogen-test

%files
/usr/bin/repogen-test

%changelog
* Tue Dec 10 2025 Repogen <test@example.com> - 1.0.0-1
- Initial release
EOF

    # Create second spec file: repogen-utils
    cat > "${RPM_BUILD_DIR}/SPECS/repogen-utils.spec" <<'EOF'
Name:           repogen-utils
Version:        2.0.0
Release:        1
Summary:        Utility package for repogen
License:        MIT
BuildArch:      x86_64

%description
This is a second test package to verify
multi-package repository generation.

%install
mkdir -p %{buildroot}/usr/bin
cat > %{buildroot}/usr/bin/repogen-utils <<'SCRIPT'
#!/bin/sh
echo "Repogen utils package v2.0.0 installed successfully!"
SCRIPT
chmod +x %{buildroot}/usr/bin/repogen-utils

%files
/usr/bin/repogen-utils

%changelog
* Tue Dec 10 2025 Repogen <test@example.com> - 2.0.0-1
- Initial release
EOF

    # Build RPMs
    rpmbuild --define "_topdir ${RPM_BUILD_DIR}" -bb "${RPM_BUILD_DIR}/SPECS/repogen-test.spec"
    rpmbuild --define "_topdir ${RPM_BUILD_DIR}" -bb "${RPM_BUILD_DIR}/SPECS/repogen-utils.spec"

    # Copy to fixtures
    cp "${RPM_BUILD_DIR}/RPMS/x86_64/repogen-test-1.0.0-1.x86_64.rpm" "${FIXTURES_DIR}/rpms/"
    cp "${RPM_BUILD_DIR}/RPMS/x86_64/repogen-utils-2.0.0-1.x86_64.rpm" "${FIXTURES_DIR}/rpms/"

    echo "✓ RPM packages created (2)"
}

# ============================================================================
# Build Alpine APK Package
# ============================================================================
build_apk() {
    echo "Building Alpine APK test packages..."

    APK_BUILD_DIR="${FIXTURES_DIR}/apk-build"
    rm -rf "${APK_BUILD_DIR}"

    # Build first package: repogen-test
    mkdir -p "${APK_BUILD_DIR}/pkg1/usr/bin"

    cat > "${APK_BUILD_DIR}/pkg1/usr/bin/repogen-test" <<'EOF'
#!/bin/sh
echo "Repogen test package v1.0.0 installed successfully!"
EOF
    chmod +x "${APK_BUILD_DIR}/pkg1/usr/bin/repogen-test"

    FILE_SIZE1=$(stat -c%s "${APK_BUILD_DIR}/pkg1/usr/bin/repogen-test" 2>/dev/null || stat -f%z "${APK_BUILD_DIR}/pkg1/usr/bin/repogen-test" 2>/dev/null)

    cat > "${APK_BUILD_DIR}/pkg1/.PKGINFO" <<EOF
# Generated by repogen test suite
pkgname = repogen-test
pkgver = 1.0.0-r0
pkgdesc = Test package for repogen
url = https://example.com
builddate = $(date +%s)
size = ${FILE_SIZE1:-100}
arch = x86_64
license = MIT
origin = repogen-test
maintainer = Repogen Test <test@example.com>
EOF

    cd "${APK_BUILD_DIR}/pkg1"
    tar -czf "${FIXTURES_DIR}/apks/repogen-test-1.0.0-r0.apk" --sort=name .PKGINFO usr/
    cd - > /dev/null

    # Build second package: repogen-utils
    mkdir -p "${APK_BUILD_DIR}/pkg2/usr/bin"

    cat > "${APK_BUILD_DIR}/pkg2/usr/bin/repogen-utils" <<'EOF'
#!/bin/sh
echo "Repogen utils package v2.0.0 installed successfully!"
EOF
    chmod +x "${APK_BUILD_DIR}/pkg2/usr/bin/repogen-utils"

    FILE_SIZE2=$(stat -c%s "${APK_BUILD_DIR}/pkg2/usr/bin/repogen-utils" 2>/dev/null || stat -f%z "${APK_BUILD_DIR}/pkg2/usr/bin/repogen-utils" 2>/dev/null)

    cat > "${APK_BUILD_DIR}/pkg2/.PKGINFO" <<EOF
# Generated by repogen test suite
pkgname = repogen-utils
pkgver = 2.0.0-r0
pkgdesc = Utility package for repogen
url = https://example.com
builddate = $(date +%s)
size = ${FILE_SIZE2:-100}
arch = x86_64
license = MIT
origin = repogen-utils
maintainer = Repogen Test <test@example.com>
EOF

    cd "${APK_BUILD_DIR}/pkg2"
    tar -czf "${FIXTURES_DIR}/apks/repogen-utils-2.0.0-r0.apk" --sort=name .PKGINFO usr/
    cd - > /dev/null

    echo "✓ Alpine APK packages created (2)"
}

# ============================================================================
# Build Homebrew Bottle
# ============================================================================
build_bottle() {
    echo "Building Homebrew bottles..."

    BOTTLE_BUILD_DIR="${FIXTURES_DIR}/bottle-build"
    rm -rf "${BOTTLE_BUILD_DIR}"

    # Build first bottle: repogen-test
    mkdir -p "${BOTTLE_BUILD_DIR}/repogen-test/1.0.0/bin"

    cat > "${BOTTLE_BUILD_DIR}/repogen-test/1.0.0/bin/repogen-test" <<'EOF'
#!/bin/sh
echo "Repogen test package v1.0.0 installed successfully!"
EOF
    chmod +x "${BOTTLE_BUILD_DIR}/repogen-test/1.0.0/bin/repogen-test"

    cd "${BOTTLE_BUILD_DIR}"
    tar czf "${FIXTURES_DIR}/bottles/repogen-test--1.0.0.x86_64_linux.bottle.tar.gz" repogen-test/
    rm -rf repogen-test

    # Build second bottle: repogen-utils
    mkdir -p "${BOTTLE_BUILD_DIR}/repogen-utils/2.0.0/bin"

    cat > "${BOTTLE_BUILD_DIR}/repogen-utils/2.0.0/bin/repogen-utils" <<'EOF'
#!/bin/sh
echo "Repogen utils package v2.0.0 installed successfully!"
EOF
    chmod +x "${BOTTLE_BUILD_DIR}/repogen-utils/2.0.0/bin/repogen-utils"

    cd "${BOTTLE_BUILD_DIR}"
    tar czf "${FIXTURES_DIR}/bottles/repogen-utils--2.0.0.x86_64_linux.bottle.tar.gz" repogen-utils/

    echo "✓ Homebrew bottles created (2)"
}

# ============================================================================
# Main
# ============================================================================

# Check for required tools
check_tool() {
    if ! command -v "$1" &> /dev/null; then
        return 1
    fi
    return 0
}

# Check if package already exists
package_exists() {
    if [ -f "$1" ]; then
        echo "✓ Package already exists: $(basename $1)"
        return 0
    fi
    return 1
}

# Build packages (check if ANY package is missing before building)
DEB_PACKAGE1="${FIXTURES_DIR}/debs/repogen-test_1.0.0_amd64.deb"
DEB_PACKAGE2="${FIXTURES_DIR}/debs/repogen-utils_2.0.0_amd64.deb"
RPM_PACKAGE1="${FIXTURES_DIR}/rpms/repogen-test-1.0.0-1.x86_64.rpm"
RPM_PACKAGE2="${FIXTURES_DIR}/rpms/repogen-utils-2.0.0-1.x86_64.rpm"
APK_PACKAGE1="${FIXTURES_DIR}/apks/repogen-test-1.0.0-r0.apk"
APK_PACKAGE2="${FIXTURES_DIR}/apks/repogen-utils-2.0.0-r0.apk"
BOTTLE_PACKAGE1="${FIXTURES_DIR}/bottles/repogen-test--1.0.0.x86_64_linux.bottle.tar.gz"
BOTTLE_PACKAGE2="${FIXTURES_DIR}/bottles/repogen-utils--2.0.0.x86_64_linux.bottle.tar.gz"

# Build Debian packages
if ! (package_exists "$DEB_PACKAGE1" && package_exists "$DEB_PACKAGE2"); then
    if check_tool dpkg-deb; then
        build_deb
    else
        echo "⚠ Skipping Debian packages (dpkg-deb not available)"
    fi
fi

# Build RPM packages
if ! (package_exists "$RPM_PACKAGE1" && package_exists "$RPM_PACKAGE2"); then
    if check_tool rpmbuild; then
        build_rpm
    else
        echo "⚠ Skipping RPM packages (rpmbuild not available)"
    fi
fi

# Build APK packages (no special tools needed)
if ! (package_exists "$APK_PACKAGE1" && package_exists "$APK_PACKAGE2"); then
    build_apk
fi

# Build Homebrew bottles (no special tools needed)
if ! (package_exists "$BOTTLE_PACKAGE1" && package_exists "$BOTTLE_PACKAGE2"); then
    build_bottle
fi

echo ""
echo "Test packages summary:"
echo "======================"
ls -lh "${FIXTURES_DIR}"/debs/*.deb 2>/dev/null || echo "  No Debian packages"
ls -lh "${FIXTURES_DIR}"/rpms/*.rpm 2>/dev/null || echo "  No RPM packages"
ls -lh "${FIXTURES_DIR}"/apks/*.apk 2>/dev/null || echo "  No Alpine packages"
ls -lh "${FIXTURES_DIR}"/bottles/*.bottle.tar.gz 2>/dev/null || echo "  No Homebrew bottles"
