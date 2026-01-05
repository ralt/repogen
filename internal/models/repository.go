package models

// RepositoryConfig contains configuration for repository generation
type RepositoryConfig struct {
	// Input/Output
	InputDir  string
	OutputDir string

	// Repository metadata
	Origin     string
	Label      string
	RepoName   string   // Repository name for Pacman .db files and optional RPM .repo naming
	Codename   string   // For Debian
	Suite      string   // For Debian
	Components []string // For Debian (main, contrib, etc.)
	Arches     []string // Architectures to support
	Version    string   // For RPM: release version (e.g., "40" for Fedora 40)

	// Signing
	GPGKeyPath    string
	GPGPassphrase string
	RSAKeyPath    string
	RSAPassphrase string
	RSAKeyName    string // For Alpine

	// Type-specific options
	BaseURL       string // For Homebrew bottles and RPM .repo files
	GPGKeyURL     string // For RPM: explicit GPG key URL (supports $releasever/$basearch variables)
	DistroVariant string // For RPM: fedora, centos, rhel (affects .repo defaults)

	// Incremental mode
	Incremental bool // Add new packages to existing repository without removing existing ones
}
