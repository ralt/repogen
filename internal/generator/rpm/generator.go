package rpm

import (
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/signer"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
)

// Generator implements the generator.Generator interface for RPM repositories
type Generator struct {
	signer signer.Signer
}

// versionArch represents a version/architecture combination
type versionArch struct {
	version string
	arch    string
}

// NewGenerator creates a new RPM generator
func NewGenerator(s signer.Signer) generator.Generator {
	return &Generator{
		signer: s,
	}
}

// Generate creates an RPM repository structure
func (g *Generator) Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error {
	logrus.Info("Generating RPM repository...")

	// Group packages by version and architecture
	versionArchPackages := make(map[versionArch][]models.Package)

	for _, pkg := range packages {
		version := getPackageVersion(config, pkg)
		arch := pkg.Architecture
		if arch == "" {
			arch = "x86_64" // default architecture
		}
		key := versionArch{version: version, arch: arch}
		versionArchPackages[key] = append(versionArchPackages[key], pkg)
	}

	// Generate repository for each version/arch combination
	for versionArchKey, pkgs := range versionArchPackages {
		if err := g.generateForVersionArch(ctx, config, versionArchKey.version, versionArchKey.arch, pkgs); err != nil {
			return fmt.Errorf("failed to generate for %s/%s: %w", versionArchKey.version, versionArchKey.arch, err)
		}
	}

	// Sign repositories if signer available (log after all versions/archs are done)
	if g.signer != nil {
		logrus.Info("Repository signed successfully")
	}

	// Generate .repo file if BaseURL is provided
	if config.BaseURL != "" {
		repoFile, err := generateRepoFile(config, g.signer != nil)
		if err != nil {
			return fmt.Errorf("failed to generate .repo file: %w", err)
		}

		// Use distro name for filename, fall back to sanitized origin
		repoFileName := fmt.Sprintf("%s.repo", getRepoFileName(config))
		repoFilePath := filepath.Join(config.OutputDir, repoFileName)

		if err := utils.WriteFile(repoFilePath, repoFile, 0644); err != nil {
			return fmt.Errorf("failed to write .repo file: %w", err)
		}

		logrus.Infof("Repository configuration file written to: %s", repoFilePath)
	}

	logrus.Infof("RPM repository generated successfully (%d packages)", len(packages))
	return nil
}

// generateForVersionArch generates repository for a specific version/arch combination
func (g *Generator) generateForVersionArch(ctx context.Context, config *models.RepositoryConfig, version, arch string, packages []models.Package) error {
	logrus.Infof("Generating for version %s, architecture: %s", version, arch)

	// Create directory structure: OutputDir/version/arch/
	versionArchDir := filepath.Join(config.OutputDir, version, arch)
	repodataDir := filepath.Join(versionArchDir, "repodata")
	packagesDir := filepath.Join(versionArchDir, "Packages")

	if err := utils.EnsureDir(repodataDir); err != nil {
		return err
	}
	if err := utils.EnsureDir(packagesDir); err != nil {
		return err
	}

	// Copy RPM files to Packages directory
	for i := range packages {
		pkg := &packages[i]
		dstPath := filepath.Join(packagesDir, filepath.Base(pkg.Filename))
		if err := utils.CopyFile(pkg.Filename, dstPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", pkg.Filename, err)
		}
		pkg.Filename = fmt.Sprintf("Packages/%s", filepath.Base(pkg.Filename))
	}

	// Generate primary.xml
	primaryXML, err := generatePrimaryXML(packages)
	if err != nil {
		return fmt.Errorf("failed to generate primary.xml: %w", err)
	}

	primaryGz, err := utils.GzipCompress(primaryXML)
	if err != nil {
		return fmt.Errorf("failed to compress primary.xml: %w", err)
	}

	primaryChecksum, err := utils.CalculateChecksum(primaryGz, "sha256")
	if err != nil {
		return err
	}

	primaryPath := filepath.Join(repodataDir, fmt.Sprintf("%s-primary.xml.gz", primaryChecksum))
	if err := utils.WriteFile(primaryPath, primaryGz, 0644); err != nil {
		return fmt.Errorf("failed to write primary.xml.gz: %w", err)
	}

	// Generate repomd.xml
	repomdXML, err := generateRepomdXML(primaryChecksum, int64(len(primaryGz)), int64(len(primaryXML)))
	if err != nil {
		return fmt.Errorf("failed to generate repomd.xml: %w", err)
	}

	repomdPath := filepath.Join(repodataDir, "repomd.xml")
	if err := utils.WriteFile(repomdPath, repomdXML, 0644); err != nil {
		return fmt.Errorf("failed to write repomd.xml: %w", err)
	}

	// Sign repomd.xml if signer available
	if g.signer != nil {
		signature, err := g.signer.SignDetached(repomdXML)
		if err != nil {
			return fmt.Errorf("failed to sign repomd.xml: %w", err)
		}

		sigPath := filepath.Join(repodataDir, "repomd.xml.asc")
		if err := utils.WriteFile(sigPath, signature, 0644); err != nil {
			return fmt.Errorf("failed to write repomd.xml.asc: %w", err)
		}
	}

	logrus.Infof("Generated repository for %s/%s (%d packages)", version, arch, len(packages))
	return nil
}

// ValidatePackages checks if packages are valid RPM packages
func (g *Generator) ValidatePackages(packages []models.Package) error {
	for _, pkg := range packages {
		if pkg.Name == "" {
			return fmt.Errorf("package missing name: %s", pkg.Filename)
		}
	}
	return nil
}

// GetSupportedType returns the package type this generator supports
func (g *Generator) GetSupportedType() scanner.PackageType {
	return scanner.TypeRpm
}

// XML structures for metadata

type metadata struct {
	XMLName       xml.Name  `xml:"metadata"`
	Xmlns         string    `xml:"xmlns,attr"`
	XmlnsRpm      string    `xml:"xmlns:rpm,attr"`
	PackagesCount int       `xml:"packages,attr"`
	Packages      []xmlPkg  `xml:"package"`
}

type xmlPkg struct {
	Type      string     `xml:"type,attr"`
	Name      string     `xml:"name"`
	Arch      string     `xml:"arch"`
	Version   xmlVersion `xml:"version"`
	Checksum  xmlChecksum `xml:"checksum"`
	Summary   string     `xml:"summary"`
	Packager  string     `xml:"packager,omitempty"`
	URL       string     `xml:"url,omitempty"`
	Time      xmlTime    `xml:"time"`
	Size      xmlSize    `xml:"size"`
	Location  xmlLocation `xml:"location"`
	Format    xmlFormat  `xml:"format"`
}

type xmlVersion struct {
	Epoch   string `xml:"epoch,attr"`
	Ver     string `xml:"ver,attr"`
	Rel     string `xml:"rel,attr"`
}

type xmlChecksum struct {
	Type  string `xml:"type,attr"`
	Pkgid string `xml:"pkgid,attr"`
	Value string `xml:",chardata"`
}

type xmlTime struct {
	File  int64 `xml:"file,attr"`
	Build int64 `xml:"build,attr"`
}

type xmlSize struct {
	Package   int64 `xml:"package,attr"`
	Installed int64 `xml:"installed,attr"`
	Archive   int64 `xml:"archive,attr"`
}

type xmlLocation struct {
	Href string `xml:"href,attr"`
}

type xmlFormat struct {
	License string   `xml:"rpm:license,omitempty"`
	Group   string   `xml:"rpm:group,omitempty"`
}

func generatePrimaryXML(packages []models.Package) ([]byte, error) {
	var xmlPackages []xmlPkg

	for _, pkg := range packages {
		release := "1"
		if r, ok := pkg.Metadata["Release"].(string); ok {
			release = r
		}

		buildTime := time.Now().Unix()
		if bt, ok := pkg.Metadata["BuildTime"].(int64); ok {
			buildTime = bt
		}

		xmlPkg := xmlPkg{
			Type: "rpm",
			Name: pkg.Name,
			Arch: pkg.Architecture,
			Version: xmlVersion{
				Epoch: "0",
				Ver:   pkg.Version,
				Rel:   release,
			},
			Checksum: xmlChecksum{
				Type:  "sha256",
				Pkgid: "YES",
				Value: pkg.SHA256Sum,
			},
			Summary:  pkg.Description,
			Packager: pkg.Maintainer,
			URL:      pkg.Homepage,
			Time: xmlTime{
				File:  time.Now().Unix(),
				Build: buildTime,
			},
			Size: xmlSize{
				Package:   pkg.Size,
				Installed: pkg.Size,
				Archive:   pkg.Size,
			},
			Location: xmlLocation{
				Href: pkg.Filename,
			},
			Format: xmlFormat{
				License: pkg.License,
				Group:   fmt.Sprintf("%v", pkg.Metadata["Group"]),
			},
		}

		xmlPackages = append(xmlPackages, xmlPkg)
	}

	meta := metadata{
		Xmlns:         "http://linux.duke.edu/metadata/common",
		XmlnsRpm:      "http://linux.duke.edu/metadata/rpm",
		PackagesCount: len(packages),
		Packages:      xmlPackages,
	}

	xmlBytes, err := xml.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), xmlBytes...), nil
}

type repomd struct {
	XMLName  xml.Name     `xml:"repomd"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Revision int64        `xml:"revision"`
	Data     []repomdData `xml:"data"`
}

type repomdData struct {
	Type         string           `xml:"type,attr"`
	Checksum     repomdChecksum   `xml:"checksum"`
	OpenChecksum repomdChecksum   `xml:"open-checksum"`
	Location     repomdLocation   `xml:"location"`
	Timestamp    int64            `xml:"timestamp"`
	Size         int64            `xml:"size"`
	OpenSize     int64            `xml:"open-size"`
}

type repomdChecksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type repomdLocation struct {
	Href string `xml:"href,attr"`
}

func generateRepomdXML(primaryChecksum string, compressedSize, uncompressedSize int64) ([]byte, error) {
	openChecksum, _ := utils.CalculateChecksum([]byte(primaryChecksum), "sha256")

	repomd := repomd{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Revision: time.Now().Unix(),
		Data: []repomdData{
			{
				Type: "primary",
				Checksum: repomdChecksum{
					Type:  "sha256",
					Value: primaryChecksum,
				},
				OpenChecksum: repomdChecksum{
					Type:  "sha256",
					Value: openChecksum,
				},
				Location: repomdLocation{
					Href: fmt.Sprintf("repodata/%s-primary.xml.gz", primaryChecksum),
				},
				Timestamp: time.Now().Unix(),
				Size:      compressedSize,
				OpenSize:  uncompressedSize,
			},
		},
	}

	xmlBytes, err := xml.MarshalIndent(repomd, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), xmlBytes...), nil
}

// generateRepoFile creates a .repo configuration file for dnf/yum
func generateRepoFile(config *models.RepositoryConfig, isSigned bool) ([]byte, error) {
	repoID := sanitizeRepoID(config.Origin)
	repoName := config.Label
	if repoName == "" {
		repoName = config.Origin
	}

	baseURL := config.BaseURL
	if baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	// Add $releasever/$basearch variables for yum/dnf substitution
	baseURL = fmt.Sprintf("%s$releasever/$basearch", baseURL)

	// Get distribution-specific defaults
	distro := config.DistroVariant
	if distro == "" {
		distro = "fedora"
	}

	gpgCheck := "0"
	repoGpgCheck := "0"
	gpgKey := ""
	additionalOptions := ""

	if isSigned {
		gpgCheck = "1"
		// Use explicit GPG key URL instead of auto-generating
		gpgKey = fmt.Sprintf("gpgkey=%s\n", config.GPGKeyURL)

		// Fedora typically enables repo_gpgcheck
		if distro == "fedora" {
			repoGpgCheck = "1"
		}
	}

	// Distribution-specific options
	switch distro {
	case "rhel":
		additionalOptions = "metadata_expire=86400"
	case "centos":
		additionalOptions = "metadata_expire=86400"
	}

	// Build the repo file content
	repoContent := fmt.Sprintf(`[%s]
name=%s
baseurl=%s
enabled=1
gpgcheck=%s`, repoID, repoName, baseURL, gpgCheck)

	if repoGpgCheck == "1" {
		repoContent += fmt.Sprintf("\nrepo_gpgcheck=%s", repoGpgCheck)
	}

	if gpgKey != "" {
		repoContent += fmt.Sprintf("\n%s", gpgKey)
	}

	if additionalOptions != "" {
		repoContent += fmt.Sprintf("\n%s", additionalOptions)
	}

	return []byte(repoContent), nil
}

// sanitizeRepoID creates a valid repository ID from a string
func sanitizeRepoID(s string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	result := ""
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			result += string(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			result += string(ch - 'A' + 'a')
		} else if ch == ' ' || ch == '_' || ch == '.' {
			result += "-"
		}
	}
	return result
}

// getPackageVersion determines the version for a package
// Priority: CLI flag -> RPM metadata -> DistroVariant default -> "40"
func getPackageVersion(config *models.RepositoryConfig, pkg models.Package) string {
	// Priority 1: Config.Version (explicit CLI flag)
	if config.Version != "" {
		return config.Version
	}

	// Priority 2: Package metadata
	if ver, ok := pkg.Metadata["DistroVersion"].(string); ok && ver != "" {
		return ver
	}

	// Priority 3: Default based on DistroVariant
	switch config.DistroVariant {
	case "fedora":
		return "40" // Latest Fedora
	case "centos":
		return "9" // CentOS Stream 9
	case "rhel":
		return "9" // RHEL 9
	default:
		return "40"
	}
}

// getRepoFileName determines the .repo filename
// Priority: DistroVariant -> Sanitized Origin (fallback)
func getRepoFileName(config *models.RepositoryConfig) string {
	// Priority 1: DistroVariant (fedora, centos, rhel)
	if config.DistroVariant != "" {
		return config.DistroVariant
	}

	// Priority 2: Sanitized Origin
	return sanitizeRepoID(config.Origin)
}
