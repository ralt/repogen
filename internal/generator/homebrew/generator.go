package homebrew

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ralt/repogen/internal/generator"
	"github.com/ralt/repogen/internal/models"
	"github.com/ralt/repogen/internal/scanner"
	"github.com/ralt/repogen/internal/utils"
	"github.com/sirupsen/logrus"
)

// Generator implements the generator.Generator interface for Homebrew taps
type Generator struct {
	baseURL string
}

// NewGenerator creates a new Homebrew generator
func NewGenerator(baseURL string) generator.Generator {
	return &Generator{
		baseURL: baseURL,
	}
}

// Generate creates a Homebrew tap structure
func (g *Generator) Generate(ctx context.Context, config *models.RepositoryConfig, packages []models.Package) error {
	logrus.Info("Generating Homebrew tap...")

	// Create directory structure
	formulaDir := filepath.Join(config.OutputDir, "Formula")
	bottlesDir := filepath.Join(config.OutputDir, "bottles")

	if err := utils.EnsureDir(formulaDir); err != nil {
		return err
	}
	if err := utils.EnsureDir(bottlesDir); err != nil {
		return err
	}

	// Group bottles by package name
	bottlesByPkg := make(map[string][]models.Package)
	for _, pkg := range packages {
		name := extractPackageName(pkg.Filename)
		bottlesByPkg[name] = append(bottlesByPkg[name], pkg)
	}

	// Generate formulas
	for pkgName, bottles := range bottlesByPkg {
		// Copy bottles
		for _, bottle := range bottles {
			dstPath := filepath.Join(bottlesDir, filepath.Base(bottle.Filename))
			if err := utils.CopyFile(bottle.Filename, dstPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", bottle.Filename, err)
			}
		}

		// Generate formula
		formula, err := g.generateFormula(pkgName, bottles)
		if err != nil {
			return fmt.Errorf("failed to generate formula for %s: %w", pkgName, err)
		}

		className := toClassName(pkgName)
		formulaPath := filepath.Join(formulaDir, fmt.Sprintf("%s.rb", pkgName))
		if err := utils.WriteFile(formulaPath, []byte(formula), 0644); err != nil {
			return fmt.Errorf("failed to write formula: %w", err)
		}

		logrus.Infof("Generated formula for %s (%s.rb)", pkgName, className)
	}

	logrus.Infof("Homebrew tap generated successfully (%d formulas)", len(bottlesByPkg))
	return nil
}

// generateFormula creates a Ruby formula file
func (g *Generator) generateFormula(name string, bottles []models.Package) (string, error) {
	className := toClassName(name)

	// Extract version from first bottle
	version := "1.0.0"
	if len(bottles) > 0 && bottles[0].Version != "" {
		version = bottles[0].Version
	}

	// Get description
	desc := fmt.Sprintf("%s package", name)
	if len(bottles) > 0 && bottles[0].Description != "" {
		desc = bottles[0].Description
	}

	// Get homepage
	homepage := "https://example.com"
	if len(bottles) > 0 && bottles[0].Homepage != "" {
		homepage = bottles[0].Homepage
	}

	// Build formula
	var formula strings.Builder

	fmt.Fprintf(&formula, "class %s < Formula\n", className)
	fmt.Fprintf(&formula, "  desc \"%s\"\n", desc)
	fmt.Fprintf(&formula, "  homepage \"%s\"\n", homepage)
	fmt.Fprintf(&formula, "  version \"%s\"\n", version)
	formula.WriteString("\n")

	// Group bottles by platform
	macosBottles := []models.Package{}
	linuxBottles := []models.Package{}

	for _, bottle := range bottles {
		platform := extractPlatform(bottle.Filename)
		if strings.Contains(platform, "linux") {
			linuxBottles = append(linuxBottles, bottle)
		} else {
			macosBottles = append(macosBottles, bottle)
		}
	}

	// macOS bottles
	if len(macosBottles) > 0 {
		formula.WriteString("  on_macos do\n")

		armBottle := findBottleForArch(macosBottles, "arm64")
		x86Bottle := findBottleForArch(macosBottles, "x86_64")

		if armBottle != nil {
			formula.WriteString("    if Hardware::CPU.arm?\n")
			url := g.getBottleURL(armBottle.Filename)
			fmt.Fprintf(&formula, "      url \"%s\"\n", url)
			fmt.Fprintf(&formula, "      sha256 \"%s\"\n", armBottle.SHA256Sum)
			formula.WriteString("    end\n")
		}

		if x86Bottle != nil {
			if armBottle != nil {
				formula.WriteString("    if Hardware::CPU.intel?\n")
			}
			url := g.getBottleURL(x86Bottle.Filename)
			fmt.Fprintf(&formula, "      url \"%s\"\n", url)
			fmt.Fprintf(&formula, "      sha256 \"%s\"\n", x86Bottle.SHA256Sum)
			if armBottle != nil {
				formula.WriteString("    end\n")
			}
		}

		formula.WriteString("  end\n")
	}

	// Linux bottles
	if len(linuxBottles) > 0 {
		formula.WriteString("\n  on_linux do\n")
		bottle := linuxBottles[0]
		url := g.getBottleURL(bottle.Filename)
		fmt.Fprintf(&formula, "    url \"%s\"\n", url)
		fmt.Fprintf(&formula, "    sha256 \"%s\"\n", bottle.SHA256Sum)
		formula.WriteString("  end\n")
	}

	formula.WriteString("end\n")

	return formula.String(), nil
}

// getBottleURL constructs the URL for a bottle
func (g *Generator) getBottleURL(filename string) string {
	if g.baseURL != "" {
		return fmt.Sprintf("%s/bottles/%s", strings.TrimRight(g.baseURL, "/"), filepath.Base(filename))
	}
	return fmt.Sprintf("bottles/%s", filepath.Base(filename))
}

// extractPackageName extracts package name from bottle filename
// Format: package--version.platform.bottle.tar.gz
func extractPackageName(filename string) string {
	base := filepath.Base(filename)
	// Remove .bottle.tar.gz or .bottle.tar
	base = strings.TrimSuffix(base, ".bottle.tar.gz")
	base = strings.TrimSuffix(base, ".bottle.tar")

	// Split by -- to get package name
	parts := strings.Split(base, "--")
	if len(parts) > 0 {
		return parts[0]
	}

	return base
}

// extractPlatform extracts platform from bottle filename
func extractPlatform(filename string) string {
	base := filepath.Base(filename)
	re := regexp.MustCompile(`--(.*)\.(bottle\.tar\.gz|bottle\.tar)`)
	matches := re.FindStringSubmatch(base)
	if len(matches) > 1 {
		return matches[1]
	}
	return "unknown"
}

// findBottleForArch finds a bottle for a specific architecture
func findBottleForArch(bottles []models.Package, arch string) *models.Package {
	for _, bottle := range bottles {
		if strings.Contains(bottle.Filename, arch) {
			return &bottle
		}
	}
	return nil
}

// toClassName converts a package name to a Ruby class name
func toClassName(name string) string {
	// Replace hyphens and underscores with spaces
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")

	// Title case each word
	words := strings.Fields(name)
	for i, word := range words {
		words[i] = strings.Title(strings.ToLower(word))
	}

	// Join without spaces
	return strings.Join(words, "")
}

// ValidatePackages checks if packages are valid Homebrew bottles
func (g *Generator) ValidatePackages(packages []models.Package) error {
	for _, pkg := range packages {
		if !strings.Contains(pkg.Filename, ".bottle.tar") {
			return fmt.Errorf("package %s is not a Homebrew bottle", pkg.Filename)
		}
	}
	return nil
}

// GetSupportedType returns the package type this generator supports
func (g *Generator) GetSupportedType() scanner.PackageType {
	return scanner.TypeHomebrewBottle
}
