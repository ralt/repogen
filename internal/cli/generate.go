package cli

import (
	"context"
	"fmt"

	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/generator/apk"
	"github.com/ralt/repogen/internal/generator/deb"
	"github.com/ralt/repogen/internal/generator/homebrew"
	"github.com/ralt/repogen/internal/generator/rpm"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/signer"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewGenerateCmd creates the generate command
func NewGenerateCmd() *cobra.Command {
	var config models.RepositoryConfig

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate repository structure",
		Long: `Scans input directory for packages and generates repository
structures with appropriate metadata files and signatures.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate configuration
			if err := validateConfig(&config); err != nil {
				return err
			}

			logrus.Info("Starting repository generation...")
			logrus.Debugf("Configuration: %+v", config)

			// Run generation
			return runGeneration(cmd.Context(), &config)
		},
	}

	// Input/Output flags
	cmd.Flags().StringVarP(&config.InputDir, "input-dir", "i", ".", "Input directory to scan")
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "./repo", "Output directory")

	// GPG signing flags (for Debian/RPM)
	cmd.Flags().StringVarP(&config.GPGKeyPath, "gpg-key", "k", "", "Path to GPG private key")
	cmd.Flags().StringVarP(&config.GPGPassphrase, "gpg-passphrase", "p", "", "GPG key passphrase")

	// RSA signing flags (for Alpine)
	cmd.Flags().StringVar(&config.RSAKeyPath, "rsa-key", "", "Path to RSA private key (for Alpine)")
	cmd.Flags().StringVar(&config.RSAPassphrase, "rsa-passphrase", "", "RSA key passphrase")
	cmd.Flags().StringVar(&config.RSAKeyName, "key-name", "repogen", "Key name for Alpine signatures")

	// Repository metadata flags
	cmd.Flags().StringVar(&config.Origin, "origin", "", "Repository origin name")
	cmd.Flags().StringVar(&config.Label, "label", "", "Repository label")
	cmd.Flags().StringVar(&config.Codename, "codename", "stable", "Codename for Debian repos")
	cmd.Flags().StringVar(&config.Suite, "suite", "", "Suite for Debian repos (defaults to codename)")
	cmd.Flags().StringSliceVar(&config.Components, "components", []string{"main"}, "Components for Debian repos")
	cmd.Flags().StringSliceVar(&config.Arches, "arch", []string{"amd64"}, "Architectures to support")

	// Type-specific options
	cmd.Flags().StringVar(&config.BaseURL, "base-url", "", "Base URL for Homebrew bottles and RPM .repo files")
	cmd.Flags().StringVar(&config.DistroVariant, "distro", "fedora", "Distribution variant for RPM repos (fedora, centos, rhel)")

	return cmd
}

func validateConfig(config *models.RepositoryConfig) error {
	if config.InputDir == "" {
		return &models.RepoGenError{
			Type: models.ErrInvalidConfig,
			Err:  fmt.Errorf("input-dir is required"),
		}
	}

	if config.OutputDir == "" {
		return &models.RepoGenError{
			Type: models.ErrInvalidConfig,
			Err:  fmt.Errorf("output-dir is required"),
		}
	}

	// Set Suite to Codename if not specified
	if config.Suite == "" {
		config.Suite = config.Codename
	}

	// Set default origin/label if not specified
	if config.Origin == "" {
		config.Origin = "Repogen Repository"
	}
	if config.Label == "" {
		config.Label = config.Origin
	}

	return nil
}

func runGeneration(ctx context.Context, config *models.RepositoryConfig) error {
	// Step 1: Scan for packages
	logrus.Infof("Scanning directory: %s", config.InputDir)
	sc := scanner.NewFileSystemScanner()
	scannedPackages, err := sc.Scan(ctx, config.InputDir)
	if err != nil {
		return &models.RepoGenError{
			Type: models.ErrFileOp,
			Err:  fmt.Errorf("failed to scan directory: %w", err),
		}
	}

	if len(scannedPackages) == 0 {
		logrus.Warn("No packages found in input directory")
		return nil
	}

	logrus.Infof("Found %d packages", len(scannedPackages))

	// Step 2: Parse packages by type
	packagesByType := make(map[scanner.PackageType][]models.Package)

	for _, scanned := range scannedPackages {
		var pkg *models.Package
		var parseErr error

		logrus.Debugf("Parsing %s package: %s", scanned.Type, scanned.Path)

		switch scanned.Type {
		case scanner.TypeDeb:
			pkg, parseErr = deb.ParsePackage(scanned.Path)
		case scanner.TypeRpm:
			pkg, parseErr = rpm.ParsePackage(scanned.Path)
		case scanner.TypeApk:
			pkg, parseErr = apk.ParsePackage(scanned.Path)
		case scanner.TypeHomebrewBottle:
			// Homebrew bottles don't need parsing, use basic info
			pkg = &models.Package{
				Filename: scanned.Path,
				Size:     scanned.Size,
			}
			// Calculate checksums
			checksums, csErr := utils.CalculateChecksums(scanned.Path)
			if csErr == nil {
				pkg.SHA256Sum = checksums.SHA256
			}
		default:
			logrus.Warnf("Unknown package type: %s", scanned.Type)
			continue
		}

		if parseErr != nil {
			logrus.Warnf("Failed to parse %s: %v", scanned.Path, parseErr)
			continue
		}

		packagesByType[scanned.Type] = append(packagesByType[scanned.Type], *pkg)
	}

	// Step 3: Initialize signers
	var gpgSigner signer.Signer
	var rsaSigner signer.RSASigner

	if config.GPGKeyPath != "" {
		gpgSigner, err = signer.NewGPGSigner(config.GPGKeyPath, config.GPGPassphrase)
		if err != nil {
			return &models.RepoGenError{
				Type: models.ErrSigning,
				Err:  fmt.Errorf("failed to initialize GPG signer: %w", err),
			}
		}
		logrus.Info("GPG signer initialized")
	}

	if config.RSAKeyPath != "" {
		rsaSigner, err = signer.NewAlpineRSASigner(config.RSAKeyPath, config.RSAPassphrase)
		if err != nil {
			return &models.RepoGenError{
				Type: models.ErrSigning,
				Err:  fmt.Errorf("failed to initialize RSA signer: %w", err),
			}
		}
		logrus.Info("RSA signer initialized")
	}

	// Step 4: Generate repositories for each type
	generators := make(map[scanner.PackageType]generator.Generator)
	generators[scanner.TypeDeb] = deb.NewGenerator(gpgSigner)
	generators[scanner.TypeRpm] = rpm.NewGenerator(gpgSigner)
	generators[scanner.TypeApk] = apk.NewGenerator(rsaSigner, config.RSAKeyName)
	generators[scanner.TypeHomebrewBottle] = homebrew.NewGenerator(config.BaseURL)

	for pkgType, packages := range packagesByType {
		gen, ok := generators[pkgType]
		if !ok {
			logrus.Warnf("No generator for package type: %s", pkgType)
			continue
		}

		logrus.Infof("Generating %s repository with %d packages...", pkgType, len(packages))

		if err := gen.ValidatePackages(packages); err != nil {
			return &models.RepoGenError{
				Type: models.ErrInvalidConfig,
				Err:  fmt.Errorf("package validation failed for %s: %w", pkgType, err),
			}
		}

		if err := gen.Generate(ctx, config, packages); err != nil {
			return &models.RepoGenError{
				Type: models.ErrMetadataGen,
				Err:  fmt.Errorf("failed to generate %s repository: %w", pkgType, err),
			}
		}
	}

	logrus.Info("Repository generation completed successfully!")
	logrus.Infof("Output directory: %s", config.OutputDir)

	return nil
}
