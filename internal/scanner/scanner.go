package scanner

import "context"

// PackageType represents the type of package
type PackageType int

const (
	TypeUnknown PackageType = iota
	TypeDeb
	TypeRpm
	TypeApk
	TypeHomebrewBottle
	TypePacman
)

// String returns the string representation of PackageType
func (pt PackageType) String() string {
	switch pt {
	case TypeDeb:
		return "deb"
	case TypeRpm:
		return "rpm"
	case TypeApk:
		return "apk"
	case TypeHomebrewBottle:
		return "brew"
	case TypePacman:
		return "pacman"
	default:
		return "unknown"
	}
}

// ScannedPackage represents a package file found during scanning
type ScannedPackage struct {
	Path string
	Type PackageType
	Size int64
}

// Scanner interface for detecting and scanning packages
type Scanner interface {
	// Scan recursively scans a directory for packages
	Scan(ctx context.Context, dir string) ([]ScannedPackage, error)

	// DetectType determines the package type of a file
	DetectType(path string) (PackageType, error)
}
