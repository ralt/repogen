package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// Magic bytes for package detection
var (
	// Debian packages start with "!<arch>\ndebian"
	debMagic = []byte("!<arch>\ndebian")

	// RPM packages start with 0xED 0xAB 0xEE 0xDB
	rpmMagic = []byte{0xED, 0xAB, 0xEE, 0xDB}

	// Gzip magic bytes (APK files are gzipped tars)
	gzipMagic = []byte{0x1F, 0x8B}

	// Zstandard magic bytes (Pacman packages .pkg.tar.zst)
	zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

	// XZ magic bytes (Pacman packages .pkg.tar.xz)
	xzMagic = []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}
)

// DetectPackageType determines the package type based on magic bytes and file extension
func DetectPackageType(path string) (PackageType, error) {
	// Open file
	f, err := os.Open(path)
	if err != nil {
		return TypeUnknown, err
	}
	defer f.Close()

	// Read first 512 bytes for magic byte detection
	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && n == 0 {
		return TypeUnknown, err
	}
	header = header[:n]

	ext := filepath.Ext(path)
	basename := filepath.Base(path)

	// Check for Debian package
	if bytes.HasPrefix(header, debMagic) || ext == ".deb" {
		return TypeDeb, nil
	}

	// Check for RPM package
	if bytes.HasPrefix(header, rpmMagic) || ext == ".rpm" {
		return TypeRpm, nil
	}

	// Check for Alpine APK (gzipped tar with .apk extension)
	if bytes.HasPrefix(header, gzipMagic) && ext == ".apk" {
		return TypeApk, nil
	}

	// Check for Pacman package (.pkg.tar.zst, .pkg.tar.xz, .pkg.tar.gz, .pkg.tar)
	if strings.Contains(basename, ".pkg.tar.") {
		// Check compression format
		if bytes.HasPrefix(header, zstdMagic) || strings.HasSuffix(basename, ".pkg.tar.zst") {
			return TypePacman, nil
		}
		if bytes.HasPrefix(header, xzMagic) || strings.HasSuffix(basename, ".pkg.tar.xz") {
			return TypePacman, nil
		}
		if bytes.HasPrefix(header, gzipMagic) && strings.HasSuffix(basename, ".pkg.tar.gz") {
			return TypePacman, nil
		}
		if strings.HasSuffix(basename, ".pkg.tar") {
			return TypePacman, nil
		}
	}

	// Check for Homebrew bottle (filename pattern)
	if strings.Contains(basename, ".bottle.tar.gz") || strings.Contains(basename, ".bottle.tar") {
		return TypeHomebrewBottle, nil
	}

	return TypeUnknown, nil
}
